package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/client"
)

const (
	defaultListenAddress = ":8787"
	tokenEnvironment     = "UPDATE_DOCKER_AGENT_TOKEN"
)

type httpServer struct {
	updater *updater
	token   string
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	token := strings.TrimSpace(os.Getenv(tokenEnvironment))
	if token == "" {
		slog.Error("update agent token is required", "environment", tokenEnvironment)
		os.Exit(1)
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		slog.Error("create Docker client", "error", err)
		os.Exit(1)
	}
	defer dockerClient.Close()

	logger := slog.Default()
	service := newUpdater(dockerClient)
	service.onComplete = func(err error) {
		if err != nil {
			logger.Error("Qingyun container update failed", "error", err)
			return
		}
		logger.Info("Qingyun container update completed")
	}
	api := &httpServer{updater: service, token: token}

	listenAddress := strings.TrimSpace(os.Getenv("UPDATE_DOCKER_AGENT_LISTEN_ADDR"))
	if listenAddress == "" {
		listenAddress = defaultListenAddress
	}

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           api.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		_ = server.Close()
	}()

	logger.Info("Qingyun Docker update agent listening", "address", listenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("update agent stopped", "error", err)
		os.Exit(1)
	}
}

func runHealthcheck() int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8787/healthz", nil)
	if err != nil {
		return 1
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func (s *httpServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/update", s.update)
	mux.HandleFunc("POST /v1/rollback", s.rollback)
	return mux
}

func (s *httpServer) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *httpServer) update(w http.ResponseWriter, r *http.Request) {
	s.queueOperation(w, r, "update")
}

func (s *httpServer) rollback(w http.ResponseWriter, r *http.Request) {
	s.queueOperation(w, r, "rollback")
}

// queueOperation accepts a server-selected release version and delegates the
// replacement to the same guarded updater for both update and rollback. The
// agent never accepts an image reference or container name from callers.
func (s *httpServer) queueOperation(w http.ResponseWriter, r *http.Request, operation string) {
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, updateResponse{Message: "unauthorized"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request updateRequest
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid update request"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid update request"})
		return
	}

	requestedVersion := strings.TrimSpace(request.TargetVersion)
	version, err := normalizeVersion(requestedVersion)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: err.Error()})
		return
	}
	if !s.updater.queue(version) {
		writeJSON(w, http.StatusConflict, updateResponse{
			Queued:        false,
			TargetVersion: requestedVersion,
			Message:       "an " + operation + " is already in progress",
		})
		return
	}

	message := "update accepted; the managed Sub2API container will restart shortly"
	if operation == "rollback" {
		message = "rollback accepted; the managed Sub2API container will restart shortly"
	}
	writeJSON(w, http.StatusAccepted, updateResponse{
		Queued:        true,
		TargetVersion: requestedVersion,
		Message:       message,
	})
}

func (s *httpServer) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	presented := strings.TrimPrefix(header, prefix)
	if len(presented) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
