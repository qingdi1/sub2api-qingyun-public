package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestNormalizeVersionRejectsImageInjection(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "0.1.158-qingyun.2", want: "0.1.158-qingyun.2", ok: true},
		{input: "v0.1.158-qingyun.2", want: "0.1.158-qingyun.2", ok: true},
		{input: "0.1.158:latest", ok: false},
		{input: "ghcr.io/other/image:latest", ok: false},
		{input: "0.1", ok: false},
		{input: "0.1.158;rm", ok: false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := normalizeVersion(test.input)
			if (err == nil) != test.ok {
				t.Fatalf("normalizeVersion(%q) error = %v, want ok=%v", test.input, err, test.ok)
			}
			if got != test.want {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestVerifyImageLabelsRequiresQingyunSourceAndVersion(t *testing.T) {
	valid := map[string]string{
		ociSourceLabel:  qingyunSourceURL,
		ociVersionLabel: "0.1.158-qingyun.2",
	}
	if err := verifyImageLabels(valid, "0.1.158-qingyun.2"); err != nil {
		t.Fatalf("valid labels rejected: %v", err)
	}

	wrongSource := mapsClone(valid)
	wrongSource[ociSourceLabel] = "https://github.com/Wei-Shaw/sub2api"
	if err := verifyImageLabels(wrongSource, "0.1.158-qingyun.2"); err == nil {
		t.Fatal("wrong OCI source was accepted")
	}

	wrongVersion := mapsClone(valid)
	wrongVersion[ociVersionLabel] = "0.1.158-qingyun.1"
	if err := verifyImageLabels(wrongVersion, "0.1.158-qingyun.2"); err == nil {
		t.Fatal("wrong OCI version was accepted")
	}
}

func TestFindTargetFiltersForManagedSub2APILabels(t *testing.T) {
	fake := &fakeDockerClient{
		containers: []container.Summary{{ID: "sub2api-id"}},
	}
	agent := newUpdater(fake)
	target, err := agent.findTarget(context.Background())
	if err != nil {
		t.Fatalf("findTarget: %v", err)
	}
	if target.ID != "sub2api-id" {
		t.Fatalf("target ID = %q", target.ID)
	}
	if len(fake.listOptions) != 1 {
		t.Fatalf("ContainerList called %d times", len(fake.listOptions))
	}
	labels := fake.listOptions[0].Filters.Get("label")
	if !slices.Contains(labels, targetLabel+"=true") || !slices.Contains(labels, targetComponentLabel+"="+targetComponent) {
		t.Fatalf("managed labels missing from filter: %v", labels)
	}
}

func TestClonedConfigurationDropsDynamicNetworkFields(t *testing.T) {
	inspect := managedInspect("sub2api-id", "/sub2api-ink", "0.1.158-qingyun.1")
	endpoint := inspect.NetworkSettings.Networks["qingyun-network"]
	endpoint.NetworkID = "dynamic-network-id"
	endpoint.EndpointID = "dynamic-endpoint-id"
	endpoint.Gateway = "172.20.0.1"
	endpoint.IPAddress = "172.20.0.9"
	endpoint.IPPrefixLen = 16
	endpoint.DNSNames = []string{"sub2api-ink", "sub2api"}
	endpoint.IPAMConfig = &network.EndpointIPAMConfig{IPv4Address: "172.20.0.9"}
	endpoint.Aliases = []string{"sub2api", "api"}

	_, _, networks, err := clonedConfiguration(inspect, imageReference("0.1.158-qingyun.2"), "0.1.158-qingyun.2")
	if err != nil {
		t.Fatalf("clonedConfiguration: %v", err)
	}
	got := networks.EndpointsConfig["qingyun-network"]
	if got == nil {
		t.Fatal("network endpoint missing")
	}
	if got.NetworkID != "" || got.EndpointID != "" || got.Gateway != "" || got.IPAddress != "" || got.IPPrefixLen != 0 || len(got.DNSNames) != 0 {
		t.Fatalf("dynamic endpoint data leaked into ContainerCreate: %#v", got)
	}
	if got.IPAMConfig == nil || got.IPAMConfig.IPv4Address != "172.20.0.9" {
		t.Fatalf("static IPAM configuration lost: %#v", got.IPAMConfig)
	}
	if !slices.Equal(got.Aliases, []string{"sub2api", "api"}) {
		t.Fatalf("aliases = %v", got.Aliases)
	}
}

func TestApplyRollsBackWhenReplacementIsUnhealthy(t *testing.T) {
	old := managedInspect("sub2api-id", "/sub2api-ink", "0.1.158-qingyun.1")
	newContainer := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    "replacement-id",
			State: &container.State{Running: true, Health: &container.Health{Status: container.Unhealthy}},
		},
	}
	fake := &fakeDockerClient{
		containers: []container.Summary{{ID: "sub2api-id"}},
		image:      verifiedImage("0.1.158-qingyun.2"),
		inspects: map[string][]container.InspectResponse{
			"sub2api-id":     {old},
			"replacement-id": {newContainer},
		},
		createID: "replacement-id",
	}
	agent := newUpdater(fake)
	agent.healthTimeout = 20 * time.Millisecond
	agent.healthPollInterval = time.Millisecond

	err := agent.apply(context.Background(), "0.1.158-qingyun.2")
	if err == nil || !strings.Contains(err.Error(), "unhealthy") {
		t.Fatalf("apply error = %v, want unhealthy replacement failure", err)
	}
	if !slices.Equal(fake.pullRefs, []string{qingyunImageRepository + ":0.1.158-qingyun.2"}) {
		t.Fatalf("unexpected image pull references: %v", fake.pullRefs)
	}
	if !slices.Contains(fake.stopCalls, "sub2api-id") || !slices.Contains(fake.stopCalls, "replacement-id") {
		t.Fatalf("expected old and replacement containers to stop, got %v", fake.stopCalls)
	}
	if !slices.Contains(fake.startCalls, "sub2api-id") {
		t.Fatalf("previous container was not restarted: %v", fake.startCalls)
	}
	if len(fake.networkDisconnectCalls) != 1 || fake.networkDisconnectCalls[0] != "qingyun-network:sub2api-id" {
		t.Fatalf("unexpected disconnect calls: %v", fake.networkDisconnectCalls)
	}
	if len(fake.networkConnectCalls) != 1 || fake.networkConnectCalls[0] != "qingyun-network:sub2api-id" {
		t.Fatalf("previous network was not restored: %v", fake.networkConnectCalls)
	}
	if !slices.Contains(fake.removeCalls, "replacement-id") {
		t.Fatalf("failed replacement was not removed: %v", fake.removeCalls)
	}
	for _, call := range append(append([]string{}, fake.stopCalls...), fake.startCalls...) {
		if strings.Contains(call, "postgres") || strings.Contains(call, "redis") {
			t.Fatalf("database or redis operation attempted: %q", call)
		}
	}
}

func TestApplyTimesOutSlowImagePullBeforeStoppingTarget(t *testing.T) {
	fake := &fakeDockerClient{
		containers: []container.Summary{{ID: "sub2api-id"}},
		imagePull: func(ctx context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	agent := newUpdater(fake)
	agent.pullTimeout = time.Millisecond
	agent.applyTimeout = time.Second

	err := agent.apply(context.Background(), "0.1.158-qingyun.2")
	if !errors.Is(err, errImagePullTimeout) {
		t.Fatalf("apply error = %v, want image pull timeout", err)
	}
	if len(fake.stopCalls) != 0 || len(fake.startCalls) != 0 {
		t.Fatalf("slow pull must not touch the managed container: stops=%v starts=%v", fake.stopCalls, fake.startCalls)
	}
}

func TestStatusRouteRequiresAuthorizationAndReportsFailures(t *testing.T) {
	fake := &fakeDockerClient{
		containers: []container.Summary{{ID: "sub2api-id"}},
		imagePull: func(context.Context, string, image.PullOptions) (io.ReadCloser, error) {
			return nil, errors.New("registry unavailable")
		},
	}
	agent := newUpdater(fake)
	server := (&httpServer{updater: agent, token: "test-token"}).routes()

	unauthorized := httptest.NewRecorder()
	server.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	if !agent.queue("0.1.158-qingyun.2", "update") {
		t.Fatal("queue should accept the first update")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		agent.mu.Lock()
		inProgress := agent.inProgress
		agent.mu.Unlock()
		if !inProgress {
			break
		}
		time.Sleep(time.Millisecond)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	server.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var status operationStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.State != operationStateFailed || status.ErrorCode != "UPDATE_FAILED" {
		t.Fatalf("status = %#v, want failed UPDATE_FAILED", status)
	}
	if status.TargetVersion != "0.1.158-qingyun.2" || status.Operation != "update" {
		t.Fatalf("status did not retain operation target: %#v", status)
	}
}

func TestConfiguredPullTimeoutBounds(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
		ok    bool
	}{
		{name: "default", input: "", want: defaultImagePullTimeout, ok: true},
		{name: "configured", input: "12m", want: 12 * time.Minute, ok: true},
		{name: "too short", input: "30s"},
		{name: "too long", input: "31m"},
		{name: "invalid", input: "forever"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := configuredPullTimeout(test.input)
			if (err == nil) != test.ok {
				t.Fatalf("configuredPullTimeout(%q) error = %v, want ok=%v", test.input, err, test.ok)
			}
			if got != test.want {
				t.Fatalf("configuredPullTimeout(%q) = %s, want %s", test.input, got, test.want)
			}
		})
	}
}

func TestRollbackRouteQueuesAuthorizedVersion(t *testing.T) {
	const targetVersion = "0.1.158-qingyun.1"
	old := managedInspect("sub2api-id", "/sub2api-ink", "0.1.158-qingyun.2")
	replacement := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    "replacement-id",
			State: &container.State{Running: true, Health: &container.Health{Status: container.Healthy}},
		},
	}
	fake := &fakeDockerClient{
		containers: []container.Summary{{ID: "sub2api-id"}},
		image:      verifiedImage(targetVersion),
		inspects: map[string][]container.InspectResponse{
			"sub2api-id":     {old},
			"replacement-id": {replacement},
		},
		createID: "replacement-id",
	}
	agent := newUpdater(fake)
	agent.healthTimeout = time.Second
	agent.healthPollInterval = time.Millisecond
	server := (&httpServer{updater: agent, token: "test-token"}).routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/rollback", strings.NewReader(`{"target_version":"v`+targetVersion+`"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("rollback status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response updateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode rollback response: %v", err)
	}
	if !response.Queued || response.TargetVersion != "v"+targetVersion {
		t.Fatalf("rollback response = %#v", response)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		agent.mu.Lock()
		inProgress := agent.inProgress
		agent.mu.Unlock()
		if !inProgress {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !slices.Equal(fake.pullRefs, []string{qingyunImageRepository + ":" + targetVersion}) {
		t.Fatalf("rollback pull references = %v", fake.pullRefs)
	}
}

func TestRollbackRouteRejectsUnauthorizedRequest(t *testing.T) {
	server := (&httpServer{updater: newUpdater(&fakeDockerClient{}), token: "test-token"}).routes()
	req := httptest.NewRequest(http.MethodPost, "/v1/rollback", strings.NewReader(`{"target_version":"0.1.158-qingyun.1"}`))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized rollback status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func managedInspect(id, name, version string) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:         id,
			Name:       name,
			HostConfig: &container.HostConfig{},
		},
		Config: &container.Config{
			Image: "ghcr.io/qingdi1/sub2api-qingyun-public:" + version,
			Labels: map[string]string{
				targetLabel:          "true",
				targetComponentLabel: targetComponent,
			},
			Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}},
		},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"qingyun-network": {Aliases: []string{"sub2api"}},
			},
		},
	}
}

func verifiedImage(version string) image.InspectResponse {
	return image.InspectResponse{
		Config: &dockerspec.DockerOCIImageConfig{
			ImageConfig: ocispec.ImageConfig{Labels: map[string]string{
				ociSourceLabel:  qingyunSourceURL,
				ociVersionLabel: version,
			}},
		},
	}
}

func mapsClone(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

type fakeDockerClient struct {
	containers []container.Summary
	image      image.InspectResponse
	inspects   map[string][]container.InspectResponse
	createID   string

	pullRefs               []string
	listOptions            []container.ListOptions
	stopCalls              []string
	startCalls             []string
	removeCalls            []string
	networkDisconnectCalls []string
	networkConnectCalls    []string
	imagePull              func(context.Context, string, image.PullOptions) (io.ReadCloser, error)
}

func (f *fakeDockerClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	f.pullRefs = append(f.pullRefs, ref)
	if f.imagePull != nil {
		return f.imagePull(ctx, ref, options)
	}
	return io.NopCloser(strings.NewReader("{}\n")), nil
}

func (f *fakeDockerClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (image.InspectResponse, error) {
	return f.image, nil
}

func (f *fakeDockerClient) ContainerList(_ context.Context, options container.ListOptions) ([]container.Summary, error) {
	f.listOptions = append(f.listOptions, options)
	return f.containers, nil
}

func (f *fakeDockerClient) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	responses := f.inspects[id]
	if len(responses) == 0 {
		return container.InspectResponse{}, fmt.Errorf("unexpected inspect of %s", id)
	}
	response := responses[0]
	if len(responses) > 1 {
		f.inspects[id] = responses[1:]
	}
	return response, nil
}

func (f *fakeDockerClient) ContainerStop(_ context.Context, id string, _ container.StopOptions) error {
	f.stopCalls = append(f.stopCalls, id)
	return nil
}

func (f *fakeDockerClient) ContainerRename(_ context.Context, _ string, _ string) error {
	return nil
}

func (f *fakeDockerClient) ContainerCreate(_ context.Context, _ *container.Config, _ *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
	return container.CreateResponse{ID: f.createID}, nil
}

func (f *fakeDockerClient) ContainerStart(_ context.Context, id string, _ container.StartOptions) error {
	f.startCalls = append(f.startCalls, id)
	return nil
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, id string, _ container.RemoveOptions) error {
	f.removeCalls = append(f.removeCalls, id)
	return nil
}

func (f *fakeDockerClient) NetworkDisconnect(_ context.Context, networkName, id string, _ bool) error {
	f.networkDisconnectCalls = append(f.networkDisconnectCalls, networkName+":"+id)
	return nil
}

func (f *fakeDockerClient) NetworkConnect(_ context.Context, networkName, id string, _ *network.EndpointSettings) error {
	f.networkConnectCalls = append(f.networkConnectCalls, networkName+":"+id)
	return nil
}

var _ DockerClient = (*fakeDockerClient)(nil)
