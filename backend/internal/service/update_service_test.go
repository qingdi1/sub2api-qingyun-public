//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

const testQingyunImageDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
}

type qingyunReleaseChannelClientStub struct {
	*updateServiceGitHubClientStub
	channelReleases []*GitHubRelease
	channelErr      error
	channelCalls    int
}

func (s *qingyunReleaseChannelClientStub) FetchQingyunReleaseChannel(context.Context, string) ([]*GitHubRelease, error) {
	s.channelCalls++
	return s.channelReleases, s.channelErr
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

type dockerUpdateAgentStub struct {
	targetVersion         string
	imageDigest           string
	rollbackTargetVersion string
	rollbackImageDigest   string
	result                *DockerUpdateAgentResult
	rollbackResult        *DockerUpdateAgentResult
	err                   error
	rollbackErr           error
}

type dockerUpdateAgentStatusStub struct {
	*dockerUpdateAgentStub
	status    *DockerUpdateAgentStatus
	statusErr error
}

func (s *dockerUpdateAgentStatusStub) GetStatus(context.Context) (*DockerUpdateAgentStatus, error) {
	return s.status, s.statusErr
}

func (s *dockerUpdateAgentStub) QueueUpdate(_ context.Context, targetVersion, imageDigest string) (*DockerUpdateAgentResult, error) {
	s.targetVersion = targetVersion
	s.imageDigest = imageDigest
	return s.result, s.err
}

func (s *dockerUpdateAgentStub) QueueRollback(_ context.Context, targetVersion, imageDigest string) (*DockerUpdateAgentResult, error) {
	s.rollbackTargetVersion = targetVersion
	s.rollbackImageDigest = imageDigest
	if s.rollbackResult != nil || s.rollbackErr != nil {
		return s.rollbackResult, s.rollbackErr
	}
	return s.result, s.err
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	_, err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func TestUpdateServicePerformUpdateQueuesDockerAgentWithServerSelectedVersion(t *testing.T) {
	agent := &dockerUpdateAgentStub{
		result: &DockerUpdateAgentResult{Queued: true, Message: "queued by test agent"},
	}
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&qingyunReleaseChannelClientStub{
			updateServiceGitHubClientStub: &updateServiceGitHubClientStub{},
			channelReleases: []*GitHubRelease{
				{TagName: "v0.1.159-qingyun.7", ImageDigest: testQingyunImageDigest},
			},
		},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		agent,
	)

	result, err := svc.PerformUpdate(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "0.1.159-qingyun.7", agent.targetVersion)
	require.Equal(t, testQingyunImageDigest, agent.imageDigest)
	require.True(t, result.Queued)
	require.False(t, result.NeedRestart)
	require.Equal(t, "0.1.159-qingyun.7", result.TargetVersion)
	require.Equal(t, UpdateDeploymentModeDockerAgent, result.DeliveryMode)
	require.Equal(t, "queued by test agent", result.Message)
}

func TestUpdateServiceDockerManualReturnsStructuredConflict(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.159-qingyun.7"}},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerManual},
		nil,
	)

	result, err := svc.PerformUpdate(context.Background())

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrDockerUpdateManualRequired)
	appErr := infraerrors.FromError(err)
	require.EqualValues(t, http.StatusConflict, appErr.Code)
	require.Equal(t, "UPDATE_DELIVERY_MANUAL_REQUIRED", appErr.Reason)
	require.Equal(t, UpdateDeploymentModeDockerManual, appErr.Metadata["delivery_mode"])
	require.Equal(t, "0.1.159-qingyun.7", appErr.Metadata["target_version"])
}

func TestUpdateServiceDockerAgentWithoutClientReturnsStructuredConflict(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&qingyunReleaseChannelClientStub{
			updateServiceGitHubClientStub: &updateServiceGitHubClientStub{},
			channelReleases: []*GitHubRelease{
				{TagName: "v0.1.159-qingyun.7", ImageDigest: testQingyunImageDigest},
			},
		},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		nil,
	)

	_, err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrDockerUpdateAgentNotConfigured)
	appErr := infraerrors.FromError(err)
	require.EqualValues(t, http.StatusConflict, appErr.Code)
	require.Equal(t, "UPDATE_AGENT_NOT_CONFIGURED", appErr.Reason)
	require.Equal(t, UpdateDeploymentModeDockerAgent, appErr.Metadata["delivery_mode"])
	require.Equal(t, "0.1.159-qingyun.7", appErr.Metadata["target_version"])
}

func TestUpdateServiceAutoSourceBuildReturnsManualDeliveryConflict(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.159-qingyun.7"}},
		"0.1.158-qingyun.2",
		"source",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeAuto},
		nil,
	)

	_, err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrDockerUpdateManualRequired)
	appErr := infraerrors.FromError(err)
	require.EqualValues(t, http.StatusConflict, appErr.Code)
	require.Equal(t, UpdateDeploymentModeDockerManual, appErr.Metadata["delivery_mode"])
}

func TestDockerUpdateAgentClientOnlyPostsServerSelectedVersion(t *testing.T) {
	const targetVersion = "0.1.159-qingyun.7"
	const token = "test-update-agent-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/update", r.URL.Path)
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))

		var request dockerUpdateAgentRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, targetVersion, request.TargetVersion)
		require.Equal(t, testQingyunImageDigest, request.ImageDigest)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queued":true,"target_version":"0.1.159-qingyun.7","message":"accepted"}`))
	}))
	defer server.Close()

	client := newDockerUpdateAgentClient(server.URL+"/v1/update", token)
	result, err := client.QueueUpdate(context.Background(), targetVersion, testQingyunImageDigest)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Queued)
	require.Equal(t, "accepted", result.Message)
}

func TestDockerUpdateAgentClientRollbackPostsDedicatedEndpoint(t *testing.T) {
	const targetVersion = "0.1.158-qingyun.1"
	const token = "test-update-agent-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/rollback", r.URL.Path)
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))

		var request dockerUpdateAgentRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, targetVersion, request.TargetVersion)
		require.Equal(t, testQingyunImageDigest, request.ImageDigest)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queued":true,"target_version":"0.1.158-qingyun.1","message":"rollback accepted"}`))
	}))
	defer server.Close()

	client := newDockerUpdateAgentClient(server.URL+"/v1/update", token)
	rollbackClient, ok := client.(DockerRollbackAgent)
	require.True(t, ok)
	result, err := rollbackClient.QueueRollback(context.Background(), targetVersion, testQingyunImageDigest)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Queued)
	require.Equal(t, "rollback accepted", result.Message)
}

func TestDockerUpdateAgentClientReadsProtectedStatus(t *testing.T) {
	const token = "test-update-agent-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/status", r.URL.Path)
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"pulling","operation":"update","target_version":"0.1.159-qingyun.7","message":"Downloading the requested image."}`))
	}))
	defer server.Close()

	client := newDockerUpdateAgentClient(server.URL+"/v1/update", token)
	statusClient, ok := client.(DockerUpdateAgentStatusClient)
	require.True(t, ok)
	status, err := statusClient.GetStatus(context.Background())

	require.NoError(t, err)
	require.Equal(t, "pulling", status.State)
	require.Equal(t, "update", status.Operation)
	require.Equal(t, "0.1.159-qingyun.7", status.TargetVersion)
}

func TestUpdateServiceGetsDockerAgentStatus(t *testing.T) {
	agent := &dockerUpdateAgentStatusStub{
		dockerUpdateAgentStub: &dockerUpdateAgentStub{},
		status: &DockerUpdateAgentStatus{
			State:         "failed",
			TargetVersion: "0.1.159-qingyun.7",
			ErrorCode:     "IMAGE_PULL_TIMEOUT",
		},
	}
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		agent,
	)

	status, err := svc.GetDockerUpdateStatus(context.Background())

	require.NoError(t, err)
	require.Equal(t, "failed", status.State)
	require.Equal(t, "IMAGE_PULL_TIMEOUT", status.ErrorCode)
}

func TestUpdateServiceStatusRequiresCompatibleAgent(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		&dockerUpdateAgentStub{},
	)

	_, err := svc.GetDockerUpdateStatus(context.Background())

	require.ErrorIs(t, err, ErrDockerUpdateStatusUnavailable)
}

func TestUpdateServiceDockerAgentRollbackQueuesAllowlistedVersion(t *testing.T) {
	const targetVersion = "0.1.158-qingyun.1"
	agent := &dockerUpdateAgentStub{
		rollbackResult: &DockerUpdateAgentResult{Queued: true, Message: "rollback queued by test agent"},
	}
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&qingyunReleaseChannelClientStub{
			updateServiceGitHubClientStub: &updateServiceGitHubClientStub{},
			channelReleases: []*GitHubRelease{
				{TagName: "v" + targetVersion, PublishedAt: "2026-07-17T00:00:00Z", ImageDigest: testQingyunImageDigest},
			},
		},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		agent,
	)

	result, err := svc.RollbackToVersionResult(context.Background(), "v"+targetVersion)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, targetVersion, agent.rollbackTargetVersion)
	require.Equal(t, testQingyunImageDigest, agent.rollbackImageDigest)
	require.True(t, result.Queued)
	require.False(t, result.NeedRestart)
	require.Equal(t, targetVersion, result.TargetVersion)
	require.Equal(t, UpdateDeploymentModeDockerAgent, result.DeliveryMode)
	require.Equal(t, "rollback queued by test agent", result.Message)
}

func TestUpdateServiceDockerAgentListsRollbackVersions(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&qingyunReleaseChannelClientStub{
			updateServiceGitHubClientStub: &updateServiceGitHubClientStub{},
			channelReleases:               []*GitHubRelease{{TagName: "v0.1.158-qingyun.1", ImageDigest: testQingyunImageDigest}},
		},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		&dockerUpdateAgentStub{},
	)

	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "0.1.158-qingyun.1", versions[0].Version)
}

func TestUpdateServiceDockerAgentUsesQingyunReleaseChannel(t *testing.T) {
	client := &qingyunReleaseChannelClientStub{
		updateServiceGitHubClientStub: &updateServiceGitHubClientStub{
			release: &GitHubRelease{TagName: "v9.9.9"},
			recentReleases: []*GitHubRelease{
				{TagName: "v9.9.9"},
			},
		},
		channelReleases: []*GitHubRelease{
			{TagName: "v0.1.158-qingyun.4", ImageDigest: testQingyunImageDigest},
			{TagName: "v0.1.158-qingyun.6", ImageDigest: testQingyunImageDigest},
			{TagName: "v0.1.158-qingyun.5", ImageDigest: testQingyunImageDigest},
			{TagName: "v0.1.158-qingyun.3", ImageDigest: testQingyunImageDigest},
		},
	}
	agent := &dockerUpdateAgentStub{
		result:         &DockerUpdateAgentResult{Queued: true, Message: "update queued"},
		rollbackResult: &DockerUpdateAgentResult{Queued: true, Message: "rollback queued"},
	}
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		client,
		"0.1.158-qingyun.5",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		agent,
	)

	update, err := svc.PerformUpdate(context.Background())
	require.NoError(t, err)
	require.Equal(t, "0.1.158-qingyun.6", update.TargetVersion)
	require.Equal(t, "0.1.158-qingyun.6", agent.targetVersion)
	require.Equal(t, testQingyunImageDigest, agent.imageDigest)

	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Equal(t, []RollbackVersion{
		{Version: "0.1.158-qingyun.4", HTMLURL: ""},
		{Version: "0.1.158-qingyun.3", HTMLURL: ""},
	}, versions)

	rollback, err := svc.RollbackToVersionResult(context.Background(), "0.1.158-qingyun.4")
	require.NoError(t, err)
	require.Equal(t, "0.1.158-qingyun.4", rollback.TargetVersion)
	require.Equal(t, "0.1.158-qingyun.4", agent.rollbackTargetVersion)
	require.Equal(t, testQingyunImageDigest, agent.rollbackImageDigest)
	require.GreaterOrEqual(t, client.channelCalls, 3)
}

func TestUpdateServiceDockerAgentEmptyQingyunReleaseChannelHasNoUpdate(t *testing.T) {
	client := &qingyunReleaseChannelClientStub{
		updateServiceGitHubClientStub: &updateServiceGitHubClientStub{},
		channelReleases:               []*GitHubRelease{},
	}
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		client,
		"0.1.158-qingyun.6",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		&dockerUpdateAgentStub{},
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, "0.1.158-qingyun.6", info.CurrentVersion)
	require.Equal(t, "0.1.158-qingyun.6", info.LatestVersion)
	require.False(t, info.HasUpdate)
	require.Nil(t, info.ReleaseInfo)
	require.Equal(t, UpdateDeploymentModeDockerAgent, info.DeliveryMode)

	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceDockerAgentMapsQingyunReleaseChannelFailure(t *testing.T) {
	client := &qingyunReleaseChannelClientStub{
		updateServiceGitHubClientStub: &updateServiceGitHubClientStub{},
		channelErr:                    errors.New("channel unavailable"),
	}
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		client,
		"0.1.158-qingyun.5",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		&dockerUpdateAgentStub{},
	)

	_, err := svc.ListRollbackVersions(context.Background())
	require.ErrorIs(t, err, ErrQingyunReleaseChannelUnavailable)
	appErr := infraerrors.FromError(err)
	require.Equal(t, "QINGYUN_RELEASE_CHANNEL_UNAVAILABLE", appErr.Reason)
}

func TestUpdateServiceDockerAgentRequiresQingyunReleaseChannelClient(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{},
		"0.1.158-qingyun.5",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerAgent},
		&dockerUpdateAgentStub{},
	)

	_, err := svc.ListRollbackVersions(context.Background())
	require.ErrorIs(t, err, ErrQingyunReleaseChannelUnavailable)
}

func TestUpdateServiceDockerRollbackNeverUsesBinaryReplacement(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: UpdateDeploymentModeDockerManual},
		nil,
	)

	require.ErrorIs(t, svc.Rollback(), ErrDockerRollbackUnsupported)
	_, err := svc.ListRollbackVersions(context.Background())
	require.ErrorIs(t, err, ErrDockerRollbackUnsupported)
	require.ErrorIs(t, svc.RollbackToVersion(context.Background(), "0.1.158-qingyun.1"), ErrDockerRollbackUnsupported)
}

func TestUpdateServiceInvalidDeliveryModeRejectsRollback(t *testing.T) {
	svc := NewUpdateServiceWithDeployment(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{},
		"0.1.158-qingyun.2",
		"release",
		UpdateDeploymentConfig{Mode: "not-a-delivery-mode"},
		nil,
	)

	_, err := svc.ListRollbackVersions(context.Background())
	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, "UPDATE_DELIVERY_MODE_INVALID", appErr.Reason)
	require.Equal(t, "not-a-delivery-mode", appErr.Metadata["delivery_mode"])

	_, err = svc.RollbackToVersionResult(context.Background(), "0.1.158-qingyun.1")
	require.Error(t, err)
	appErr = infraerrors.FromError(err)
	require.Equal(t, "UPDATE_DELIVERY_MODE_INVALID", appErr.Reason)
	require.Equal(t, "0.1.158-qingyun.1", appErr.Metadata["target_version"])
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}
