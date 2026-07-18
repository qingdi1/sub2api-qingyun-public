package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	qingyunImageRepository = "ghcr.io/qingdi1/sub2api-qingyun-public"
	targetLabel            = "io.qingyun.sub2api.update-target"
	targetComponentLabel   = "io.qingyun.sub2api.component"
	targetComponent        = "sub2api"
	updatedVersionLabel    = "io.qingyun.sub2api.version"
	updatedImageLabel      = "io.qingyun.sub2api.image"
	ociSourceLabel         = "org.opencontainers.image.source"
	ociVersionLabel        = "org.opencontainers.image.version"
	qingyunSourceURL       = "https://github.com/qingdi1/sub2api-qingyun-public"

	defaultImagePullTimeout = 20 * time.Minute
	defaultApplyTimeout     = 5 * time.Minute
)

var versionPattern = regexp.MustCompile(`^v?([0-9]+(?:\.[0-9]+){2}(?:-[0-9A-Za-z][0-9A-Za-z._-]*)?)$`)

var imageDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

var errImagePullTimeout = errors.New("image pull timed out")

const (
	operationStateIdle           = "idle"
	operationStateQueued         = "queued"
	operationStatePulling        = "pulling"
	operationStateVerifying      = "verifying"
	operationStateReplacing      = "replacing"
	operationStateWaitingHealthy = "waiting_healthy"
	operationStateSucceeded      = "succeeded"
	operationStateFailed         = "failed"
)

// DockerClient contains the only Docker operations the update agent is allowed
// to perform. Keeping the interface narrow makes the container boundary
// auditable and lets tests prove that Postgres and Redis are never targeted.
type DockerClient interface {
	ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error)
	ImageInspect(context.Context, string, ...client.ImageInspectOption) (image.InspectResponse, error)
	ContainerList(context.Context, container.ListOptions) ([]container.Summary, error)
	ContainerInspect(context.Context, string) (container.InspectResponse, error)
	ContainerStop(context.Context, string, container.StopOptions) error
	ContainerRename(context.Context, string, string) error
	ContainerCreate(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *v1.Platform, string) (container.CreateResponse, error)
	ContainerStart(context.Context, string, container.StartOptions) error
	ContainerRemove(context.Context, string, container.RemoveOptions) error
	NetworkDisconnect(context.Context, string, string, bool) error
	NetworkConnect(context.Context, string, string, *network.EndpointSettings) error
}

type updateRequest struct {
	TargetVersion string `json:"target_version"`
	ImageDigest   string `json:"image_digest"`
}

type updateResponse struct {
	Queued        bool   `json:"queued"`
	TargetVersion string `json:"target_version"`
	Message       string `json:"message"`
}

// operationStatus is deliberately small and free of Docker details. It is
// retained in-memory by the update control plane so the application can show
// meaningful progress or failure feedback without ever exposing its token to
// the browser.
type operationStatus struct {
	State         string    `json:"state"`
	Operation     string    `json:"operation,omitempty"`
	TargetVersion string    `json:"target_version,omitempty"`
	Message       string    `json:"message,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type updater struct {
	docker DockerClient

	mu                 sync.Mutex
	inProgress         bool
	onComplete         func(error)
	pullTimeout        time.Duration
	applyTimeout       time.Duration
	healthTimeout      time.Duration
	healthPollInterval time.Duration
	status             operationStatus
}

func newUpdater(dockerClient DockerClient) *updater {
	return &updater{
		docker:             dockerClient,
		pullTimeout:        defaultImagePullTimeout,
		applyTimeout:       defaultApplyTimeout,
		healthTimeout:      90 * time.Second,
		healthPollInterval: time.Second,
		status: operationStatus{
			State:     operationStateIdle,
			UpdatedAt: time.Now().UTC(),
		},
	}
}

func normalizeVersion(value string) (string, error) {
	value = strings.TrimSpace(value)
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return "", fmt.Errorf("target_version must be a release version")
	}
	return match[1], nil
}

func normalizeImageDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !imageDigestPattern.MatchString(value) {
		return "", fmt.Errorf("image_digest must be a SHA-256 OCI digest")
	}
	return value, nil
}

func imageReference(imageDigest string) string {
	return qingyunImageRepository + "@" + imageDigest
}

func (u *updater) queue(version, imageDigest, operation string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.inProgress {
		return false
	}
	u.inProgress = true
	u.status = operationStatus{
		State:         operationStateQueued,
		Operation:     operation,
		TargetVersion: version,
		Message:       "Update request accepted and waiting for the Docker worker.",
		UpdatedAt:     time.Now().UTC(),
	}
	go func() {
		err := u.apply(context.Background(), version, imageDigest)
		if u.onComplete != nil {
			u.onComplete(err)
		}
		u.mu.Lock()
		if err != nil {
			u.status = failedOperationStatus(u.status, err)
		} else {
			u.status.State = operationStateSucceeded
			u.status.Message = "The requested version is healthy and ready."
			u.status.ErrorCode = ""
			u.status.UpdatedAt = time.Now().UTC()
		}
		u.inProgress = false
		u.mu.Unlock()
	}()
	return true
}

func (u *updater) apply(ctx context.Context, version, imageDigest string) (err error) {
	targetCtx, cancelTarget := context.WithTimeout(ctx, u.effectiveApplyTimeout())
	target, err := u.findTarget(targetCtx)
	cancelTarget()
	if err != nil {
		return err
	}

	ref := imageReference(imageDigest)
	u.setStage(operationStatePulling, "Downloading the requested image.")
	pullCtx, cancelPull := context.WithTimeout(ctx, u.effectivePullTimeout())
	pullStream, err := u.docker.ImagePull(pullCtx, ref, image.PullOptions{})
	if err != nil {
		cancelPull()
		if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s", errImagePullTimeout, ref)
		}
		return fmt.Errorf("pull Qingyun image %q: %w", ref, err)
	}
	if err := consumePullProgress(pullStream); err != nil {
		cancelPull()
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s", errImagePullTimeout, ref)
		}
		return fmt.Errorf("pull Qingyun image %q: %w", ref, err)
	}
	cancelPull()

	applyCtx, cancelApply := context.WithTimeout(ctx, u.effectiveApplyTimeout())
	defer cancelApply()
	u.setStage(operationStateVerifying, "Verifying the published image.")
	if err := u.verifyTargetImage(applyCtx, ref, version, imageDigest); err != nil {
		return err
	}

	inspect, err := u.docker.ContainerInspect(applyCtx, target.ID)
	if err != nil {
		return fmt.Errorf("inspect managed sub2api container: %w", err)
	}
	if !isManagedSub2API(inspect.Config) {
		return errors.New("managed sub2api label changed while update was pending")
	}
	if !hasHealthcheck(inspect.Config) {
		return errors.New("managed sub2api container must define a Docker healthcheck before it can be updated")
	}

	config, hostConfig, networkingConfig, err := clonedConfiguration(inspect, ref, version)
	if err != nil {
		return err
	}

	originalName := strings.TrimPrefix(inspect.Name, "/")
	if originalName == "" {
		return errors.New("managed sub2api container has no name")
	}
	backupName := rollbackName(originalName)
	stopTimeout := 30

	// Keep the previous container intact until the replacement reaches Docker's
	// healthy state. If creation, networking, start, or health verification
	// fails, recovery restores it under the original name. Only the managed
	// application container is ever stopped or disconnected.
	stopped := false
	renamed := false
	newID := ""
	disconnectedNetworks := make([]string, 0, len(networkingConfig.EndpointsConfig))
	completed := false
	defer func() {
		if completed {
			return
		}
		if restoreErr := u.restorePreviousContainer(target.ID, originalName, newID, stopped, renamed, disconnectedNetworks, networkingConfig, stopTimeout); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore previous managed sub2api container: %w", restoreErr))
		}
	}()

	u.setStage(operationStateReplacing, "Replacing the application container.")
	if err := u.docker.ContainerStop(applyCtx, target.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		return fmt.Errorf("stop managed sub2api container: %w", err)
	}
	stopped = true

	if err := u.docker.ContainerRename(applyCtx, target.ID, backupName); err != nil {
		return fmt.Errorf("preserve managed sub2api container for rollback: %w", err)
	}
	renamed = true

	for _, networkName := range sortedNetworkNames(networkingConfig) {
		if err := u.docker.NetworkDisconnect(applyCtx, networkName, target.ID, false); err != nil {
			return fmt.Errorf("disconnect previous sub2api container from network %q: %w", networkName, err)
		}
		disconnectedNetworks = append(disconnectedNetworks, networkName)
	}

	created, err := u.docker.ContainerCreate(applyCtx, config, hostConfig, networkingConfig, nil, originalName)
	if err != nil {
		return fmt.Errorf("create updated sub2api container: %w", err)
	}
	newID = created.ID

	if err := u.docker.ContainerStart(applyCtx, newID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start updated sub2api container: %w", err)
	}
	u.setStage(operationStateWaitingHealthy, "Waiting for the replacement container to become healthy.")
	if err := u.waitForHealthy(applyCtx, newID); err != nil {
		return fmt.Errorf("updated sub2api container did not become healthy: %w", err)
	}
	if err := u.docker.ContainerRemove(applyCtx, target.ID, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("remove rollback sub2api container: %w", err)
	}

	completed = true
	return nil
}

func (u *updater) effectivePullTimeout() time.Duration {
	if u.pullTimeout <= 0 {
		return defaultImagePullTimeout
	}
	return u.pullTimeout
}

func (u *updater) effectiveApplyTimeout() time.Duration {
	if u.applyTimeout <= 0 {
		return defaultApplyTimeout
	}
	return u.applyTimeout
}

func (u *updater) setStage(state, message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status.State = state
	u.status.Message = message
	u.status.ErrorCode = ""
	u.status.UpdatedAt = time.Now().UTC()
}

func (u *updater) statusSnapshot() operationStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func failedOperationStatus(current operationStatus, err error) operationStatus {
	current.State = operationStateFailed
	current.UpdatedAt = time.Now().UTC()
	if errors.Is(err, errImagePullTimeout) {
		current.ErrorCode = "IMAGE_PULL_TIMEOUT"
		current.Message = "The image download timed out. Check registry connectivity and retry."
		return current
	}
	current.ErrorCode = "UPDATE_FAILED"
	current.Message = "The update agent could not complete the requested operation. Check its logs and retry."
	return current
}

func (u *updater) verifyTargetImage(ctx context.Context, ref, version, imageDigest string) error {
	inspect, err := u.docker.ImageInspect(ctx, ref)
	if err != nil {
		return fmt.Errorf("inspect pulled Qingyun image %q: %w", ref, err)
	}
	if inspect.Config == nil || inspect.Config.Labels == nil {
		return fmt.Errorf("pulled Qingyun image %q has no OCI labels", ref)
	}
	if err := verifyImageLabels(inspect.Config.Labels, version); err != nil {
		return fmt.Errorf("pulled Qingyun image %q: %w", ref, err)
	}
	if !slices.Contains(inspect.RepoDigests, imageReference(imageDigest)) {
		return fmt.Errorf("pulled Qingyun image %q does not retain the expected digest", ref)
	}
	return nil
}

func verifyImageLabels(labels map[string]string, version string) error {
	if labels[ociSourceLabel] != qingyunSourceURL {
		return fmt.Errorf("unexpected OCI source %q", labels[ociSourceLabel])
	}
	labelVersion, err := normalizeVersion(labels[ociVersionLabel])
	if err != nil || labelVersion != version {
		return fmt.Errorf("unexpected OCI version %q", labels[ociVersionLabel])
	}
	return nil
}

func hasHealthcheck(config *container.Config) bool {
	return config != nil && config.Healthcheck != nil && len(config.Healthcheck.Test) > 0 && config.Healthcheck.Test[0] != "NONE"
}

func (u *updater) waitForHealthy(ctx context.Context, containerID string) error {
	timeout := u.healthTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	pollInterval := u.healthPollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		inspect, err := u.docker.ContainerInspect(healthCtx, containerID)
		if err != nil {
			return fmt.Errorf("inspect replacement: %w", err)
		}
		if inspect.State == nil || !inspect.State.Running {
			return errors.New("replacement is not running")
		}
		if inspect.State.Health == nil {
			return errors.New("replacement has no Docker health state")
		}
		switch inspect.State.Health.Status {
		case container.Healthy:
			return nil
		case container.Unhealthy:
			return errors.New("replacement reported unhealthy")
		}

		select {
		case <-healthCtx.Done():
			return fmt.Errorf("timed out waiting for healthy state: %w", healthCtx.Err())
		case <-time.After(pollInterval):
		}
	}
}

func (u *updater) restorePreviousContainer(targetID, originalName, newID string, stopped, renamed bool, disconnectedNetworks []string, networkingConfig *network.NetworkingConfig, stopTimeout int) error {
	var recoveryErrors []error
	if newID != "" {
		if err := u.docker.ContainerStop(context.Background(), newID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("stop failed replacement: %w", err))
		}
		if err := u.docker.ContainerRemove(context.Background(), newID, container.RemoveOptions{Force: true}); err != nil {
			return errors.Join(append(recoveryErrors, fmt.Errorf("remove failed replacement: %w", err))...)
		}
	}
	for _, networkName := range disconnectedNetworks {
		endpoint := networkingConfig.EndpointsConfig[networkName]
		if err := u.docker.NetworkConnect(context.Background(), networkName, targetID, endpoint); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("reconnect previous container to network %q: %w", networkName, err))
		}
	}
	if renamed {
		if err := u.docker.ContainerRename(context.Background(), targetID, originalName); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("restore previous container name: %w", err))
		}
	}
	if stopped {
		if err := u.docker.ContainerStart(context.Background(), targetID, container.StartOptions{}); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("restart previous container: %w", err))
		}
	}
	return errors.Join(recoveryErrors...)
}

func (u *updater) findTarget(ctx context.Context) (container.Summary, error) {
	args := filters.NewArgs(
		filters.Arg("label", targetLabel+"=true"),
		filters.Arg("label", targetComponentLabel+"="+targetComponent),
	)
	containers, err := u.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return container.Summary{}, fmt.Errorf("list managed sub2api containers: %w", err)
	}
	if len(containers) != 1 {
		return container.Summary{}, fmt.Errorf("expected exactly one managed sub2api container, found %d", len(containers))
	}
	return containers[0], nil
}

func isManagedSub2API(config *container.Config) bool {
	if config == nil || config.Labels == nil {
		return false
	}
	return config.Labels[targetLabel] == "true" && config.Labels[targetComponentLabel] == targetComponent
}

func clonedConfiguration(inspect container.InspectResponse, ref, version string) (*container.Config, *container.HostConfig, *network.NetworkingConfig, error) {
	if inspect.Config == nil || inspect.HostConfig == nil {
		return nil, nil, nil, errors.New("managed sub2api container has incomplete Docker configuration")
	}
	config, err := cloneJSON(inspect.Config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("clone sub2api container config: %w", err)
	}
	hostConfig, err := cloneJSON(inspect.HostConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("clone sub2api host config: %w", err)
	}

	config.Image = ref
	config.Labels = maps.Clone(config.Labels)
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}
	config.Labels[updatedVersionLabel] = version
	config.Labels[updatedImageLabel] = ref

	networkingConfig, err := sanitizedNetworkingConfig(inspect.NetworkSettings)
	if err != nil {
		return nil, nil, nil, err
	}
	return config, hostConfig, networkingConfig, nil
}

// sanitizedNetworkingConfig retains user-configured endpoint settings but
// deliberately drops inspect-only IDs, allocated addresses, gateways, and DNS
// names. The old target is disconnected before the replacement is created, so
// aliases and static IPAM configuration can be safely restored on rollback.
func sanitizedNetworkingConfig(settings *container.NetworkSettings) (*network.NetworkingConfig, error) {
	config := &network.NetworkingConfig{EndpointsConfig: make(map[string]*network.EndpointSettings)}
	if settings == nil {
		return config, nil
	}
	for networkName, endpoint := range settings.Networks {
		if endpoint == nil {
			return nil, fmt.Errorf("sub2api network %q has no endpoint settings", networkName)
		}
		config.EndpointsConfig[networkName] = &network.EndpointSettings{
			IPAMConfig: cloneIPAMConfig(endpoint.IPAMConfig),
			Links:      slices.Clone(endpoint.Links),
			Aliases:    slices.Clone(endpoint.Aliases),
			MacAddress: endpoint.MacAddress,
			DriverOpts: maps.Clone(endpoint.DriverOpts),
			GwPriority: endpoint.GwPriority,
		}
	}
	return config, nil
}

func cloneIPAMConfig(input *network.EndpointIPAMConfig) *network.EndpointIPAMConfig {
	if input == nil {
		return nil
	}
	return &network.EndpointIPAMConfig{
		IPv4Address:  input.IPv4Address,
		IPv6Address:  input.IPv6Address,
		LinkLocalIPs: slices.Clone(input.LinkLocalIPs),
	}
}

func sortedNetworkNames(config *network.NetworkingConfig) []string {
	if config == nil || len(config.EndpointsConfig) == 0 {
		return nil
	}
	names := make([]string, 0, len(config.EndpointsConfig))
	for name := range config.EndpointsConfig {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cloneJSON[T any](input T) (T, error) {
	var output T
	data, err := json.Marshal(input)
	if err != nil {
		return output, err
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return output, err
	}
	return output, nil
}

func consumePullProgress(stream io.ReadCloser) error {
	defer stream.Close()
	decoder := json.NewDecoder(stream)
	for {
		var progress struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&progress); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if progress.Error != "" {
			return errors.New(progress.Error)
		}
	}
}

func rollbackName(name string) string {
	suffix := fmt.Sprintf("-qingyun-rollback-%d", time.Now().UnixNano())
	const maxDockerNameLength = 255
	if len(name)+len(suffix) > maxDockerNameLength {
		name = name[:maxDockerNameLength-len(suffix)]
	}
	return name + suffix
}

var _ DockerClient = (*client.Client)(nil)
