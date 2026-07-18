package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type githubReleaseClient struct {
	httpClient         *http.Client
	downloadHTTPClient *http.Client
}

type githubReleaseClientError struct {
	err error
}

var releaseFallbackVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

var qingyunReleaseChannelVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-qingyun\.[0-9]+$`)

var qingyunReleaseChannelDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

var releaseFallbackBranches = []string{"qingyun-chat", "main"}

var releaseFallbackVersionFiles = []string{"VERSION", "backend/cmd/server/VERSION"}

const (
	qingyunReleaseChannelBranch     = "qingyun-chat"
	qingyunReleaseChannelPath       = ".qingyun/release-channel.json"
	maxQingyunReleaseChannelBytes   = 64 * 1024
	maxQingyunReleaseChannelEntries = 50
)

type qingyunReleaseChannelDocument struct {
	SchemaVersion int                            `json:"schema_version"`
	Releases      []qingyunReleaseChannelRelease `json:"releases"`
}

type qingyunReleaseChannelRelease struct {
	Version     string `json:"version"`
	PublishedAt string `json:"published_at"`
	ImageDigest string `json:"image_digest"`
}

// NewGitHubReleaseClient 创建 GitHub Release 客户端
// proxyURL 为空时直连 GitHub，支持 http/https/socks5/socks5h 协议
// 代理配置失败时行为由 allowDirectOnProxyError 控制：
//   - false（默认）：返回错误占位客户端，禁止回退到直连
//   - true：回退到直连（仅限管理员显式开启）
func NewGitHubReleaseClient(proxyURL string, allowDirectOnProxyError bool) service.GitHubReleaseClient {
	// 安全说明：httpclient.GetClient 的错误链（url.Parse / proxyutil）不含明文代理凭据，
	// 但仍通过 slog 仅在服务端日志记录，不会暴露给 HTTP 响应。
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:  30 * time.Second,
		ProxyURL: proxyURL,
	})
	if err != nil {
		if strings.TrimSpace(proxyURL) != "" && !allowDirectOnProxyError {
			slog.Warn("proxy client init failed, all requests will fail", "service", "github_release", "error", err)
			return &githubReleaseClientError{err: fmt.Errorf("proxy client init failed and direct fallback is disabled; set security.proxy_fallback.allow_direct_on_error=true to allow fallback: %w", err)}
		}
		sharedClient = &http.Client{Timeout: 30 * time.Second}
	}

	// 下载客户端需要更长的超时时间
	downloadClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:  10 * time.Minute,
		ProxyURL: proxyURL,
	})
	if err != nil {
		if strings.TrimSpace(proxyURL) != "" && !allowDirectOnProxyError {
			slog.Warn("proxy download client init failed, all requests will fail", "service", "github_release", "error", err)
			return &githubReleaseClientError{err: fmt.Errorf("proxy client init failed and direct fallback is disabled; set security.proxy_fallback.allow_direct_on_error=true to allow fallback: %w", err)}
		}
		downloadClient = &http.Client{Timeout: 10 * time.Minute}
	}

	return &githubReleaseClient{
		httpClient:         sharedClient,
		downloadHTTPClient: downloadClient,
	}
}

func (c *githubReleaseClientError) FetchLatestRelease(ctx context.Context, repo string) (*service.GitHubRelease, error) {
	return nil, c.err
}

func (c *githubReleaseClientError) FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*service.GitHubRelease, error) {
	return nil, c.err
}

func (c *githubReleaseClientError) FetchQingyunReleaseChannel(ctx context.Context, repo string) ([]*service.GitHubRelease, error) {
	return nil, c.err
}

func (c *githubReleaseClientError) DownloadFile(ctx context.Context, url, dest string, maxSize int64) error {
	return c.err
}

func (c *githubReleaseClientError) FetchChecksumFile(ctx context.Context, url string) ([]byte, error) {
	return nil, c.err
}

func (c *githubReleaseClient) FetchLatestRelease(ctx context.Context, repo string) (*service.GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Sub2API-Updater")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return c.fetchLatestVersionFallback(ctx, repo)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release service.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// fetchLatestVersionFallback supports repositories that publish a container and
// keep a version file on the maintained branch before creating GitHub Releases.
func (c *githubReleaseClient) fetchLatestVersionFallback(ctx context.Context, repo string) (*service.GitHubRelease, error) {
	var lastErr error
	for _, branch := range releaseFallbackBranches {
		for _, versionFile := range releaseFallbackVersionFiles {
			version, found, err := c.fetchBranchVersion(ctx, repo, branch, versionFile)
			if err != nil {
				lastErr = err
				continue
			}
			if !found {
				continue
			}

			return &service.GitHubRelease{
				TagName: "v" + version,
				Name:    "v" + version,
				Body: fmt.Sprintf(
					"Version detected from %s on the %s branch. Container image: ghcr.io/%s:%s",
					versionFile,
					branch,
					repo,
					version,
				),
				HTMLURL: fmt.Sprintf("https://github.com/%s/blob/%s/%s", repo, branch, versionFile),
				Assets:  []service.GitHubAsset{},
			}, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("GitHub release not found and version fallback failed: %w", lastErr)
	}
	return nil, fmt.Errorf("GitHub release and version files were not found for %s", repo)
}

func (c *githubReleaseClient) fetchBranchVersion(ctx context.Context, repo, branch, versionFile string) (string, bool, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo, branch, versionFile)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "Sub2API-Updater")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("GitHub raw content returned %d for %s/%s", resp.StatusCode, branch, versionFile)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 257))
	if err != nil {
		return "", false, err
	}
	version := strings.TrimSpace(string(data))
	if len(version) > 64 || !releaseFallbackVersionPattern.MatchString(version) {
		return "", false, fmt.Errorf("invalid version in %s/%s", branch, versionFile)
	}

	return strings.TrimPrefix(version, "v"), true, nil
}

func (c *githubReleaseClient) FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*service.GitHubRelease, error) {
	if perPage <= 0 {
		perPage = 10
	}
	if perPage > 100 {
		perPage = 100 // GitHub API hard limit
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d", repo, perPage)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Sub2API-Updater")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []*service.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

// FetchQingyunReleaseChannel reads the public, branch-pinned Qingyun release
// channel. Docker update and rollback selection uses this explicit allowlist
// instead of GitHub's rate-limited anonymous REST release API.
func (c *githubReleaseClient) FetchQingyunReleaseChannel(ctx context.Context, repo string) ([]*service.GitHubRelease, error) {
	url := fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s",
		repo,
		qingyunReleaseChannelBranch,
		qingyunReleaseChannelPath,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sub2API-Updater")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Qingyun release channel returned %d", resp.StatusCode)
	}
	if resp.ContentLength > maxQingyunReleaseChannelBytes {
		return nil, fmt.Errorf("Qingyun release channel is too large: %d bytes", resp.ContentLength)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQingyunReleaseChannelBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxQingyunReleaseChannelBytes {
		return nil, fmt.Errorf("Qingyun release channel exceeds %d bytes", maxQingyunReleaseChannelBytes)
	}

	var document qingyunReleaseChannelDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode Qingyun release channel: %w", err)
	}
	if document.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported Qingyun release channel schema %d", document.SchemaVersion)
	}
	if len(document.Releases) == 0 || len(document.Releases) > maxQingyunReleaseChannelEntries {
		return nil, fmt.Errorf("Qingyun release channel must contain 1 to %d releases", maxQingyunReleaseChannelEntries)
	}

	releases := make([]*service.GitHubRelease, 0, len(document.Releases))
	seen := make(map[string]struct{}, len(document.Releases))
	for _, entry := range document.Releases {
		version := strings.TrimSpace(entry.Version)
		if !qingyunReleaseChannelVersionPattern.MatchString(version) {
			return nil, fmt.Errorf("invalid Qingyun release channel version %q", version)
		}
		if _, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate Qingyun release channel version %q", version)
		}
		if publishedAt := strings.TrimSpace(entry.PublishedAt); publishedAt != "" {
			if _, err := time.Parse(time.RFC3339, publishedAt); err != nil {
				return nil, fmt.Errorf("invalid publication time for Qingyun version %q: %w", version, err)
			}
		}
		imageDigest := strings.TrimSpace(entry.ImageDigest)
		if !qingyunReleaseChannelDigestPattern.MatchString(imageDigest) {
			return nil, fmt.Errorf("invalid image digest for Qingyun version %q", version)
		}
		seen[version] = struct{}{}
		releases = append(releases, &service.GitHubRelease{
			TagName:     "v" + version,
			Name:        "v" + version,
			PublishedAt: strings.TrimSpace(entry.PublishedAt),
			HTMLURL:     fmt.Sprintf("https://github.com/%s/releases/tag/v%s", repo, version),
			ImageDigest: imageDigest,
		})
	}

	return releases, nil
}

func (c *githubReleaseClient) DownloadFile(ctx context.Context, url, dest string, maxSize int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// 使用预配置的下载客户端（已包含代理配置）
	resp, err := c.downloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	// SECURITY: Check Content-Length if available
	if resp.ContentLength > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", resp.ContentLength, maxSize)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}

	// SECURITY: Use LimitReader to enforce max download size even if Content-Length is missing/wrong
	limited := io.LimitReader(resp.Body, maxSize+1)
	written, err := io.Copy(out, limited)

	// Close file before attempting to remove (required on Windows)
	_ = out.Close()

	if err != nil {
		_ = os.Remove(dest) // Clean up partial file (best-effort)
		return err
	}

	// Check if we hit the limit (downloaded more than maxSize)
	if written > maxSize {
		_ = os.Remove(dest) // Clean up partial file (best-effort)
		return fmt.Errorf("download exceeded maximum size of %d bytes", maxSize)
	}

	return nil
}

func (c *githubReleaseClient) FetchChecksumFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
