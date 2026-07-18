package service

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrNoUpdateAvailable          = infraerrors.Conflict("ALREADY_UP_TO_DATE", "no update available; current version is latest")
	ErrRollbackVersionNotAllowed  = infraerrors.BadRequest("ROLLBACK_VERSION_NOT_ALLOWED", "version is not in the allowed rollback list")
	ErrDockerUpdateManualRequired = infraerrors.Conflict(
		"UPDATE_DELIVERY_MANUAL_REQUIRED",
		"This deployment cannot update itself. Configure the Docker update agent or update the image with Docker Compose.",
	)
	ErrDockerUpdateAgentNotConfigured = infraerrors.Conflict(
		"UPDATE_AGENT_NOT_CONFIGURED",
		"Docker update delivery is selected, but no Docker update agent is configured.",
	)
	ErrDockerRollbackAgentNotConfigured = infraerrors.Conflict(
		"ROLLBACK_AGENT_NOT_CONFIGURED",
		"Docker rollback delivery is selected, but no Docker update agent is configured.",
	)
	ErrDockerRollbackUnsupported = infraerrors.Conflict(
		"DOCKER_ROLLBACK_UNSUPPORTED",
		"Rollback from this Docker deployment is not available through the binary updater. Pin the desired image version with Docker Compose.",
	)
	ErrDockerUpdateStatusUnavailable = infraerrors.ServiceUnavailable(
		"UPDATE_AGENT_STATUS_UNAVAILABLE",
		"The Docker update agent status could not be reached. Verify the update-agent service and try again.",
	)
)

const (
	updateCacheKey = "update_check_cache"
	updateCacheTTL = 1200 // 20 minutes
	githubRepo     = "qingdi1/sub2api-qingyun-public"

	UpdateDeploymentModeAuto         = "auto"
	UpdateDeploymentModeBinary       = "binary"
	UpdateDeploymentModeDockerAgent  = "docker-agent"
	UpdateDeploymentModeDockerManual = "docker-manual"

	// Security: allowed download domains for updates
	allowedDownloadHost = "github.com"
	allowedAssetHost    = "objects.githubusercontent.com"
	allowedGHCRHost     = "ghcr.io"
	allowedPkgHost      = "pkg-containers.githubusercontent.com"

	// Security: max download size (500MB)
	maxDownloadSize = 500 * 1024 * 1024

	// Rollback: expose at most the 3 most recent versions older than current
	maxRollbackVersions = 3
	// Fetch a few extra releases so filtering (current/newer/prerelease) still leaves enough candidates
	rollbackFetchPageSize = 15

	dockerUpdateAgentMaxResponseBytes = 64 * 1024
	dockerUpdateAgentRequestTimeout   = 15 * time.Second
)

// UpdateCache defines cache operations for update service
type UpdateCache interface {
	GetUpdateInfo(ctx context.Context) (string, error)
	SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error
}

// GitHubReleaseClient 获取 GitHub release 信息的接口
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*GitHubRelease, error)
	DownloadFile(ctx context.Context, url, dest string, maxSize int64) error
	FetchChecksumFile(ctx context.Context, url string) ([]byte, error)
}

// UpdateDeploymentConfig is the server-side delivery configuration for released updates.
// It intentionally contains no image name or container identifier: those are owned by the
// update agent deployment, rather than exposed through the administrator-facing API.
type UpdateDeploymentConfig struct {
	Mode             string
	DockerAgentURL   string
	DockerAgentToken string
}

// DockerUpdateAgent is a narrow client for a local, operator-configured Docker updater.
// The service supplies the release version after verifying it against the configured release
// channel. Callers cannot provide an image or container target.
type DockerUpdateAgent interface {
	QueueUpdate(ctx context.Context, targetVersion string) (*DockerUpdateAgentResult, error)
}

// DockerRollbackAgent is implemented by agents that expose the rollback
// operation separately from normal updates. Keeping this as a separate
// interface preserves compatibility with existing update-only test doubles and
// makes it impossible to accidentally use the update endpoint for rollback.
type DockerRollbackAgent interface {
	QueueRollback(ctx context.Context, targetVersion string) (*DockerUpdateAgentResult, error)
}

// DockerUpdateAgentStatusClient is optional so existing update-only agent
// integrations remain compatible while newer agents can report asynchronous
// progress and failure states back to the administrator UI.
type DockerUpdateAgentStatusClient interface {
	GetStatus(ctx context.Context) (*DockerUpdateAgentStatus, error)
}

// DockerUpdateAgentResult is the accepted asynchronous update request returned by the agent.
type DockerUpdateAgentResult struct {
	Queued  bool   `json:"queued"`
	Message string `json:"message,omitempty"`
}

// DockerUpdateAgentStatus is the sanitized progress snapshot returned by the
// private update control plane. Browser clients only receive it through the
// authenticated administrator API; they never know the agent endpoint/token.
type DockerUpdateAgentStatus struct {
	State         string `json:"state"`
	Operation     string `json:"operation,omitempty"`
	TargetVersion string `json:"target_version,omitempty"`
	Message       string `json:"message,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type dockerUpdateAgentClient struct {
	endpoint         string
	rollbackEndpoint string
	statusEndpoint   string
	token            string
	client           *http.Client
}

type dockerUpdateAgentRequest struct {
	TargetVersion string `json:"target_version"`
}

type dockerUpdateAgentResponse struct {
	Queued        bool   `json:"queued"`
	TargetVersion string `json:"target_version,omitempty"`
	Message       string `json:"message,omitempty"`
}

// UpdateService handles software updates
type UpdateService struct {
	cache          UpdateCache
	githubClient   GitHubReleaseClient
	currentVersion string
	buildType      string // "source" for manual builds, "release" for CI builds
	deployment     UpdateDeploymentConfig
	dockerAgent    DockerUpdateAgent
}

// NewUpdateService creates a new UpdateService
func NewUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, version, buildType string) *UpdateService {
	return NewUpdateServiceWithDeployment(cache, githubClient, version, buildType, UpdateDeploymentConfig{
		Mode: UpdateDeploymentModeBinary,
	}, nil)
}

// NewUpdateServiceWithDeployment creates an UpdateService with an explicit release delivery mode.
// The default constructor remains binary-only for existing callers and tests; the application wire
// provider supplies the runtime auto/docker configuration.
func NewUpdateServiceWithDeployment(
	cache UpdateCache,
	githubClient GitHubReleaseClient,
	version, buildType string,
	deployment UpdateDeploymentConfig,
	dockerAgent DockerUpdateAgent,
) *UpdateService {
	if dockerAgent == nil && strings.TrimSpace(deployment.DockerAgentURL) != "" {
		dockerAgent = newDockerUpdateAgentClient(deployment.DockerAgentURL, deployment.DockerAgentToken)
	}
	return &UpdateService{
		cache:          cache,
		githubClient:   githubClient,
		currentVersion: version,
		buildType:      buildType,
		deployment:     deployment,
		dockerAgent:    dockerAgent,
	}
}

// UpdateInfo contains update information
type UpdateInfo struct {
	CurrentVersion string       `json:"current_version"`
	LatestVersion  string       `json:"latest_version"`
	HasUpdate      bool         `json:"has_update"`
	ReleaseInfo    *ReleaseInfo `json:"release_info,omitempty"`
	Cached         bool         `json:"cached"`
	Warning        string       `json:"warning,omitempty"`
	BuildType      string       `json:"build_type"` // "source" or "release"
	DeliveryMode   string       `json:"delivery_mode"`
}

// UpdateResult is returned after an update has either been applied to a binary
// or accepted by the Docker update agent. Docker agent updates are asynchronous,
// so queued is true and need_restart stays false.
type UpdateResult struct {
	Message       string `json:"message"`
	NeedRestart   bool   `json:"need_restart"`
	Queued        bool   `json:"queued"`
	TargetVersion string `json:"target_version"`
	DeliveryMode  string `json:"delivery_mode"`
}

// ReleaseInfo contains GitHub release details
type ReleaseInfo struct {
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Assets      []Asset `json:"assets,omitempty"`
}

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
}

// GitHubRelease represents GitHub API response
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
}

// RollbackVersion describes a release version the system can roll back to
type RollbackVersion struct {
	Version     string `json:"version"` // without "v" prefix, e.g. "0.1.146"
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckUpdate checks for available updates
func (s *UpdateService) CheckUpdate(ctx context.Context, force bool) (*UpdateInfo, error) {
	// Try cache first
	if !force {
		if cached, err := s.getFromCache(ctx); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Fetch from GitHub
	info, err := s.fetchLatestRelease(ctx)
	if err != nil {
		// Return cached on error
		if cached, cacheErr := s.getFromCache(ctx); cacheErr == nil && cached != nil {
			cached.Warning = "Using cached data: " + err.Error()
			return cached, nil
		}
		return &UpdateInfo{
			CurrentVersion: s.currentVersion,
			LatestVersion:  s.currentVersion,
			HasUpdate:      false,
			Warning:        err.Error(),
			BuildType:      s.buildType,
			DeliveryMode:   s.deliveryMode(),
		}, nil
	}

	// Cache result
	s.saveToCache(ctx, info)
	return info, nil
}

// PerformUpdate selects the configured deployment delivery and applies or queues the update.
// Container deployments never fall through to applyReleaseAssets: image/container selection is
// intentionally delegated to a separately configured Docker update agent.
func (s *UpdateService) PerformUpdate(ctx context.Context) (*UpdateResult, error) {
	info, err := s.CheckUpdate(ctx, true)
	if err != nil {
		return nil, err
	}

	if !info.HasUpdate {
		return nil, ErrNoUpdateAvailable
	}

	mode := s.deliveryMode()
	targetVersion := info.LatestVersion
	switch mode {
	case UpdateDeploymentModeBinary:
		if err := s.applyReleaseAssets(ctx, info.ReleaseInfo.Assets); err != nil {
			return nil, err
		}
		return &UpdateResult{
			Message:       "Update completed. Please restart the service.",
			NeedRestart:   true,
			Queued:        false,
			TargetVersion: targetVersion,
			DeliveryMode:  mode,
		}, nil
	case UpdateDeploymentModeDockerAgent:
		if s.dockerAgent == nil {
			return nil, s.deliveryError(ErrDockerUpdateAgentNotConfigured, mode, targetVersion)
		}
		agentResult, err := s.dockerAgent.QueueUpdate(ctx, targetVersion)
		if err != nil {
			return nil, s.dockerAgentError(mode, targetVersion, err)
		}
		if agentResult == nil || !agentResult.Queued {
			return nil, s.deliveryError(
				infraerrors.ServiceUnavailable("UPDATE_AGENT_REJECTED", "The Docker update agent did not accept the update request."),
				mode,
				targetVersion,
			)
		}
		message := strings.TrimSpace(agentResult.Message)
		if message == "" {
			message = "Docker update queued. The service will restart after the new image is ready."
		}
		return &UpdateResult{
			Message:       message,
			NeedRestart:   false,
			Queued:        true,
			TargetVersion: targetVersion,
			DeliveryMode:  mode,
		}, nil
	case UpdateDeploymentModeDockerManual:
		return nil, s.deliveryError(ErrDockerUpdateManualRequired, mode, targetVersion)
	default:
		return nil, s.deliveryError(
			infraerrors.Conflict("UPDATE_DELIVERY_MODE_INVALID", "The configured update delivery mode is invalid."),
			mode,
			targetVersion,
		)
	}
}

// GetDockerUpdateStatus returns the latest asynchronous operation state from
// the configured Docker control plane. It deliberately exposes no image or
// container identifier, and is only used by the authenticated admin handler.
func (s *UpdateService) GetDockerUpdateStatus(ctx context.Context) (*DockerUpdateAgentStatus, error) {
	mode := s.deliveryMode()
	if mode != UpdateDeploymentModeDockerAgent {
		return nil, s.deliveryError(ErrDockerUpdateStatusUnavailable, mode, "")
	}
	statusClient, ok := s.dockerAgent.(DockerUpdateAgentStatusClient)
	if !ok || statusClient == nil {
		return nil, s.deliveryError(ErrDockerUpdateStatusUnavailable, mode, "")
	}
	status, err := statusClient.GetStatus(ctx)
	if err != nil {
		return nil, s.deliveryError(ErrDockerUpdateStatusUnavailable, mode, "").WithCause(err)
	}
	return status, nil
}

func (s *UpdateService) deliveryMode() string {
	configured := strings.ToLower(strings.TrimSpace(s.deployment.Mode))
	switch configured {
	case "", UpdateDeploymentModeAuto:
		hasConfiguredAgent := strings.TrimSpace(s.deployment.DockerAgentURL) != "" &&
			strings.TrimSpace(s.deployment.DockerAgentToken) != ""
		if hasConfiguredAgent || (s.dockerAgent != nil && strings.TrimSpace(s.deployment.DockerAgentURL) == "") {
			return UpdateDeploymentModeDockerAgent
		}
		if isContainerDeployment() || !strings.EqualFold(strings.TrimSpace(s.buildType), "release") {
			return UpdateDeploymentModeDockerManual
		}
		return UpdateDeploymentModeBinary
	case UpdateDeploymentModeBinary, UpdateDeploymentModeDockerAgent, UpdateDeploymentModeDockerManual:
		return configured
	default:
		return configured
	}
}

func isContainerDeployment() bool {
	if strings.TrimSpace(os.Getenv("SUB2API_CONTAINERIZED")) == "true" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// /.dockerenv is not present for every runtime (notably some Kubernetes and
	// rootless Docker setups), so use the cgroup marker as a second, read-only hint.
	cgroup, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	value := strings.ToLower(string(cgroup))
	return strings.Contains(value, "docker") ||
		strings.Contains(value, "containerd") ||
		strings.Contains(value, "kubepods") ||
		strings.Contains(value, "podman")
}

func (s *UpdateService) deliveryError(base *infraerrors.ApplicationError, mode, targetVersion string) *infraerrors.ApplicationError {
	return base.WithMetadata(map[string]string{
		"delivery_mode":  mode,
		"target_version": targetVersion,
	})
}

func (s *UpdateService) dockerAgentError(mode, targetVersion string, err error) error {
	base := infraerrors.ServiceUnavailable(
		"UPDATE_AGENT_UNAVAILABLE",
		"The Docker update agent could not be reached. Verify the update-agent service and try again.",
	)
	return s.deliveryError(base, mode, targetVersion).WithCause(err)
}

func (s *UpdateService) dockerRollbackAgentError(mode, targetVersion string, err error) error {
	base := infraerrors.ServiceUnavailable(
		"ROLLBACK_AGENT_UNAVAILABLE",
		"The Docker update agent could not be reached for rollback. Verify the update-agent service and try again.",
	)
	return s.deliveryError(base, mode, targetVersion).WithCause(err)
}

func newDockerUpdateAgentClient(endpoint, token string) DockerUpdateAgent {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	rollbackEndpoint := endpoint + "/rollback"
	statusEndpoint := endpoint + "/status"
	if strings.HasSuffix(endpoint, "/update") {
		rollbackEndpoint = strings.TrimSuffix(endpoint, "/update") + "/rollback"
		statusEndpoint = strings.TrimSuffix(endpoint, "/update") + "/status"
	}
	return &dockerUpdateAgentClient{
		endpoint:         endpoint,
		rollbackEndpoint: rollbackEndpoint,
		statusEndpoint:   statusEndpoint,
		token:            strings.TrimSpace(token),
		client: &http.Client{
			Timeout: dockerUpdateAgentRequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (a *dockerUpdateAgentClient) QueueUpdate(ctx context.Context, targetVersion string) (*DockerUpdateAgentResult, error) {
	return a.queueAt(ctx, a.endpoint, targetVersion)
}

func (a *dockerUpdateAgentClient) QueueRollback(ctx context.Context, targetVersion string) (*DockerUpdateAgentResult, error) {
	return a.queueAt(ctx, a.rollbackEndpoint, targetVersion)
}

func (a *dockerUpdateAgentClient) GetStatus(ctx context.Context) (*DockerUpdateAgentStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.statusEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build docker update status request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sub2API-Docker-Updater")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send docker update status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, dockerUpdateAgentMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read docker update status response: %w", err)
	}
	if len(body) > dockerUpdateAgentMaxResponseBytes {
		return nil, fmt.Errorf("docker update status response exceeds %d bytes", dockerUpdateAgentMaxResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("docker update agent status returned HTTP %d", resp.StatusCode)
	}

	var status DockerUpdateAgentStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("decode docker update status response: %w", err)
	}
	if strings.TrimSpace(status.State) == "" {
		return nil, errors.New("docker update agent returned an empty status")
	}
	return &status, nil
}

func (a *dockerUpdateAgentClient) queueAt(ctx context.Context, endpoint, targetVersion string) (*DockerUpdateAgentResult, error) {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return nil, fmt.Errorf("target version is required")
	}
	payload, err := json.Marshal(dockerUpdateAgentRequest{TargetVersion: targetVersion})
	if err != nil {
		return nil, fmt.Errorf("encode docker update request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build docker update request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sub2API-Docker-Updater")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send docker update request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, dockerUpdateAgentMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read docker update response: %w", err)
	}
	if len(body) > dockerUpdateAgentMaxResponseBytes {
		return nil, fmt.Errorf("docker update response exceeds %d bytes", dockerUpdateAgentMaxResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("docker update agent returned HTTP %d", resp.StatusCode)
	}

	var decoded dockerUpdateAgentResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode docker update response: %w", err)
	}
	if reported := strings.TrimSpace(decoded.TargetVersion); reported != "" && reported != targetVersion {
		return nil, fmt.Errorf("docker update agent acknowledged unexpected target version %q", reported)
	}
	return &DockerUpdateAgentResult{
		Queued:  decoded.Queued,
		Message: strings.TrimSpace(decoded.Message),
	}, nil
}

// applyReleaseAssets downloads the platform archive from the given release assets,
// verifies its checksum, and atomically swaps the running binary.
// Shared by PerformUpdate (latest) and RollbackToVersion (specific older version).
func (s *UpdateService) applyReleaseAssets(ctx context.Context, releaseAssets []Asset) error {
	// Find matching archive and checksum for current platform
	archiveName := s.getArchiveName()
	var downloadURL string
	var checksumURL string

	for _, asset := range releaseAssets {
		if strings.Contains(asset.Name, archiveName) && !strings.HasSuffix(asset.Name, ".txt") {
			downloadURL = asset.DownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumURL = asset.DownloadURL
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no compatible release found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// SECURITY: Validate download URL is from trusted domain
	if err := validateDownloadURL(downloadURL); err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	if checksumURL != "" {
		if err := validateDownloadURL(checksumURL); err != nil {
			return fmt.Errorf("invalid checksum URL: %w", err)
		}
	}

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	exeDir := filepath.Dir(exePath)

	// Create temp directory in the SAME directory as executable
	// This ensures os.Rename is atomic (same filesystem)
	tempDir, err := os.MkdirTemp(exeDir, ".sub2api-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Download archive
	archivePath := filepath.Join(tempDir, filepath.Base(downloadURL))
	if err := s.downloadFile(ctx, downloadURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum if available
	if checksumURL != "" {
		if err := s.verifyChecksum(ctx, archivePath, checksumURL); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Extract binary from archive
	newBinaryPath := filepath.Join(tempDir, "sub2api")
	if err := s.extractBinary(archivePath, newBinaryPath); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Set executable permission before replacement
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Atomic replacement using rename pattern:
	// 1. Rename current -> backup (atomic on Unix)
	// 2. Rename new -> current (atomic on Unix, same filesystem)
	// If step 2 fails, restore backup
	backupPath := exePath + ".backup"

	// Remove old backup if exists
	_ = os.Remove(backupPath)

	// Step 1: Move current binary to backup
	if err := os.Rename(exePath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Step 2: Move new binary to target location (atomic, same filesystem)
	if err := os.Rename(newBinaryPath, exePath); err != nil {
		// Restore backup on failure
		if restoreErr := os.Rename(backupPath, exePath); restoreErr != nil {
			return fmt.Errorf("replace failed and restore failed: %w (restore error: %v)", err, restoreErr)
		}
		return fmt.Errorf("replace failed (restored backup): %w", err)
	}

	// Success - backup file is kept for rollback capability
	// It will be cleaned up on next successful update
	return nil
}

// Rollback restores the previous version
func (s *UpdateService) Rollback() error {
	if mode := s.deliveryMode(); mode != UpdateDeploymentModeBinary {
		return s.deliveryError(ErrDockerRollbackUnsupported, mode, "")
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	backupFile := exePath + ".backup"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("no backup found")
	}

	// Replace current with backup
	if err := os.Rename(backupFile, exePath); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	return nil
}

// ListRollbackVersions returns up to maxRollbackVersions release versions that are
// strictly older than the current version (the current version itself is excluded),
// newest first. Draft and prerelease entries are skipped.
func (s *UpdateService) ListRollbackVersions(ctx context.Context) ([]RollbackVersion, error) {
	mode := s.deliveryMode()
	switch mode {
	case UpdateDeploymentModeBinary, UpdateDeploymentModeDockerAgent:
	case UpdateDeploymentModeDockerManual:
		return nil, s.deliveryError(ErrDockerRollbackUnsupported, mode, "")
	default:
		return nil, s.deliveryError(
			infraerrors.Conflict("UPDATE_DELIVERY_MODE_INVALID", "The configured update delivery mode is invalid."),
			mode,
			"",
		)
	}
	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return nil, err
	}

	versions := make([]RollbackVersion, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, RollbackVersion{
			Version:     strings.TrimPrefix(r.TagName, "v"),
			PublishedAt: r.PublishedAt,
			HTMLURL:     r.HTMLURL,
		})
	}
	return versions, nil
}

// RollbackToVersion downloads and installs a specific older version, preserving
// the legacy error-only API used by callers that do not need asynchronous
// delivery details. Docker-agent deployments use RollbackToVersionResult and
// queue the image replacement through the dedicated rollback endpoint.
func (s *UpdateService) RollbackToVersion(ctx context.Context, version string) error {
	_, err := s.RollbackToVersionResult(ctx, version)
	return err
}

// RollbackToVersionResult validates an allowlisted older release and either
// installs its binary (binary deployments) or queues the corresponding image
// rollback (docker-agent deployments). The server, never the browser, selects
// the final version and the agent owns the image/container target.
func (s *UpdateService) RollbackToVersionResult(ctx context.Context, version string) (*UpdateResult, error) {
	mode := s.deliveryMode()
	target := strings.TrimPrefix(strings.TrimSpace(version), "v")
	switch mode {
	case UpdateDeploymentModeBinary, UpdateDeploymentModeDockerAgent:
	case UpdateDeploymentModeDockerManual:
		return nil, s.deliveryError(ErrDockerRollbackUnsupported, mode, strings.TrimPrefix(strings.TrimSpace(version), "v"))
	default:
		return nil, s.deliveryError(
			infraerrors.Conflict("UPDATE_DELIVERY_MODE_INVALID", "The configured update delivery mode is invalid."),
			mode,
			target,
		)
	}
	if target == "" {
		return nil, ErrRollbackVersionNotAllowed
	}

	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return nil, err
	}

	var match *GitHubRelease
	for _, r := range releases {
		if strings.TrimPrefix(r.TagName, "v") == target {
			match = r
			break
		}
	}
	if match == nil {
		return nil, ErrRollbackVersionNotAllowed
	}

	if mode == UpdateDeploymentModeDockerAgent {
		rollbackAgent, ok := s.dockerAgent.(DockerRollbackAgent)
		if !ok || rollbackAgent == nil {
			return nil, s.deliveryError(ErrDockerRollbackAgentNotConfigured, mode, target)
		}
		agentResult, err := rollbackAgent.QueueRollback(ctx, target)
		if err != nil {
			return nil, s.dockerRollbackAgentError(mode, target, err)
		}
		if agentResult == nil || !agentResult.Queued {
			return nil, s.deliveryError(
				infraerrors.ServiceUnavailable("ROLLBACK_AGENT_REJECTED", "The Docker update agent did not accept the rollback request."),
				mode,
				target,
			)
		}
		message := strings.TrimSpace(agentResult.Message)
		if message == "" {
			message = "Docker rollback queued. The service will restart after the target image is ready."
		}
		return &UpdateResult{
			Message:       message,
			NeedRestart:   false,
			Queued:        true,
			TargetVersion: target,
			DeliveryMode:  mode,
		}, nil
	}

	assets := make([]Asset, len(match.Assets))
	for i, a := range match.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		}
	}

	if err := s.applyReleaseAssets(ctx, assets); err != nil {
		return nil, err
	}
	return &UpdateResult{
		Message:       "Rollback completed. Please restart the service.",
		NeedRestart:   true,
		Queued:        false,
		TargetVersion: target,
		DeliveryMode:  mode,
	}, nil
}

// fetchRollbackCandidates fetches recent releases and keeps the newest
// maxRollbackVersions entries strictly older than the current version.
func (s *UpdateService) fetchRollbackCandidates(ctx context.Context) ([]*GitHubRelease, error) {
	releases, err := s.githubClient.FetchRecentReleases(ctx, githubRepo, rollbackFetchPageSize)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(releases))
	candidates := make([]*GitHubRelease, 0, maxRollbackVersions)
	for _, r := range releases {
		if r == nil || r.Draft || r.Prerelease {
			continue
		}
		v := strings.TrimPrefix(r.TagName, "v")
		if v == "" || seen[v] {
			continue
		}
		// Only versions strictly older than current (also excludes current itself)
		if compareVersions(v, s.currentVersion) >= 0 {
			continue
		}
		seen[v] = true
		candidates = append(candidates, r)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersions(
			strings.TrimPrefix(candidates[i].TagName, "v"),
			strings.TrimPrefix(candidates[j].TagName, "v"),
		) > 0
	})

	if len(candidates) > maxRollbackVersions {
		candidates = candidates[:maxRollbackVersions]
	}
	return candidates, nil
}

func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*UpdateInfo, error) {
	release, err := s.githubClient.FetchLatestRelease(ctx, githubRepo)
	if err != nil {
		return nil, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	assets := make([]Asset, len(release.Assets))
	for i, a := range release.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		}
	}

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      compareVersions(s.currentVersion, latestVersion) < 0,
		ReleaseInfo: &ReleaseInfo{
			Name:        release.Name,
			Body:        release.Body,
			PublishedAt: release.PublishedAt,
			HTMLURL:     release.HTMLURL,
			Assets:      assets,
		},
		Cached:       false,
		BuildType:    s.buildType,
		DeliveryMode: s.deliveryMode(),
	}, nil
}

func (s *UpdateService) downloadFile(ctx context.Context, downloadURL, dest string) error {
	return s.githubClient.DownloadFile(ctx, downloadURL, dest, maxDownloadSize)
}

func (s *UpdateService) getArchiveName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s_%s", osName, arch)
}

// validateDownloadURL checks if the URL is from an allowed domain
// SECURITY: This prevents SSRF and ensures downloads only come from trusted GitHub domains
func validateDownloadURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Must be HTTPS
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed")
	}

	// Check against allowed hosts
	host := parsedURL.Host
	// Trusted sources: GitHub releases, GitHub assets, and our GHCR package endpoints.
	if host != allowedDownloadHost &&
		!strings.HasSuffix(host, "."+allowedDownloadHost) &&
		host != allowedAssetHost &&
		!strings.HasSuffix(host, "."+allowedAssetHost) &&
		host != allowedGHCRHost &&
		!strings.HasSuffix(host, "."+allowedGHCRHost) &&
		host != allowedPkgHost &&
		!strings.HasSuffix(host, "."+allowedPkgHost) {
		return fmt.Errorf("download from untrusted host: %s", host)
	}

	return nil
}

func (s *UpdateService) verifyChecksum(ctx context.Context, filePath, checksumURL string) error {
	// Download checksums file
	checksumData, err := s.githubClient.FetchChecksumFile(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}

	// Calculate file hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Find expected hash in checksums file
	fileName := filepath.Base(filePath)
	scanner := bufio.NewScanner(strings.NewReader(string(checksumData)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == fileName {
			if parts[0] == actualHash {
				return nil
			}
			return fmt.Errorf("checksum mismatch: expected %s, got %s", parts[0], actualHash)
		}
	}

	return fmt.Errorf("checksum not found for %s", fileName)
}

func (s *UpdateService) extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader = f

	// Handle gzip compression
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() { _ = gzr.Close() }()
		reader = gzr
	}

	// Handle tar archive
	if strings.Contains(archivePath, ".tar") {
		tr := tar.NewReader(reader)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			// SECURITY: Prevent Zip Slip / Path Traversal attack
			// Only allow files with safe base names, no directory traversal
			baseName := filepath.Base(hdr.Name)

			// Check for path traversal attempts
			if strings.Contains(hdr.Name, "..") {
				return fmt.Errorf("path traversal attempt detected: %s", hdr.Name)
			}

			// Validate the entry is a regular file
			if hdr.Typeflag != tar.TypeReg {
				continue // Skip directories and special files
			}

			// Only extract the specific binary we need
			if baseName == "sub2api" || baseName == "sub2api.exe" {
				// Additional security: limit file size (max 500MB)
				const maxBinarySize = 500 * 1024 * 1024
				if hdr.Size > maxBinarySize {
					return fmt.Errorf("binary too large: %d bytes (max %d)", hdr.Size, maxBinarySize)
				}

				out, err := os.Create(destPath)
				if err != nil {
					return err
				}

				// Use LimitReader to prevent decompression bombs
				limited := io.LimitReader(tr, maxBinarySize)
				if _, err := io.Copy(out, limited); err != nil {
					_ = out.Close()
					return err
				}
				if err := out.Close(); err != nil {
					return err
				}
				return nil
			}
		}
		return fmt.Errorf("binary not found in archive")
	}

	// Direct copy for non-tar files (with size limit)
	const maxBinarySize = 500 * 1024 * 1024
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}

	limited := io.LimitReader(reader, maxBinarySize)
	if _, err := io.Copy(out, limited); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (s *UpdateService) getFromCache(ctx context.Context) (*UpdateInfo, error) {
	data, err := s.cache.GetUpdateInfo(ctx)
	if err != nil {
		return nil, err
	}

	var cached struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, err
	}

	if time.Now().Unix()-cached.Timestamp > updateCacheTTL {
		return nil, fmt.Errorf("cache expired")
	}

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  cached.Latest,
		HasUpdate:      compareVersions(s.currentVersion, cached.Latest) < 0,
		ReleaseInfo:    cached.ReleaseInfo,
		Cached:         true,
		BuildType:      s.buildType,
		DeliveryMode:   s.deliveryMode(),
	}, nil
}

func (s *UpdateService) saveToCache(ctx context.Context, info *UpdateInfo) {
	cacheData := struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
	}{
		Latest:      info.LatestVersion,
		ReleaseInfo: info.ReleaseInfo,
		Timestamp:   time.Now().Unix(),
	}

	data, _ := json.Marshal(cacheData)
	_ = s.cache.SetUpdateInfo(ctx, string(data), time.Duration(updateCacheTTL)*time.Second)
}

// compareVersions compares two semantic versions
func compareVersions(current, latest string) int {
	currentParts := parseVersion(current)
	latestParts := parseVersion(latest)

	for i := 0; i < len(currentParts); i++ {
		if currentParts[i] < latestParts[i] {
			return -1
		}
		if currentParts[i] > latestParts[i] {
			return 1
		}
	}
	return 0
}

func parseVersion(v string) [4]int {
	v = strings.TrimPrefix(v, "v")
	core, qingyunRevision, hasQingyunRevision := strings.Cut(v, "-qingyun.")
	parts := strings.Split(core, ".")
	result := [4]int{0, 0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		part := parts[i]
		if suffix := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); suffix >= 0 {
			part = part[:suffix]
		}
		if parsed, err := strconv.Atoi(part); err == nil {
			result[i] = parsed
		}
	}
	if hasQingyunRevision {
		if suffix := strings.IndexFunc(qingyunRevision, func(r rune) bool { return r < '0' || r > '9' }); suffix >= 0 {
			qingyunRevision = qingyunRevision[:suffix]
		}
		if parsed, err := strconv.Atoi(qingyunRevision); err == nil {
			result[3] = parsed
		}
	}
	return result
}
