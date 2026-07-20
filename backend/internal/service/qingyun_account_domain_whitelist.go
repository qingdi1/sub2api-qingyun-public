package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const qingyunAccountDomainWhitelistURL = "https://raw.githubusercontent.com/qingdi1/sub2api-qingyun-public/qingyun-chat/frontend/public/account-domain-whitelist.json"

// QingyunAccountDomainWhitelist is deliberately loaded from the public Qingyun
// repository. It remains outside instance images so operators can change it
// without a container rollout.
type QingyunAccountDomainWhitelist struct {
	Version int      `json:"version"`
	Domains []string `json:"domains"`
	IPs     []string `json:"ips"`
}

type QingyunAccountDomainWhitelistService struct {
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	value       QingyunAccountDomainWhitelist
	etag        string
	lastChecked time.Time
	hasValue    bool
}

func NewQingyunAccountDomainWhitelistService() *QingyunAccountDomainWhitelistService {
	return &QingyunAccountDomainWhitelistService{
		client: &http.Client{Timeout: 8 * time.Second},
		now:    time.Now,
	}
}

func (s *QingyunAccountDomainWhitelistService) Get(ctx context.Context) (QingyunAccountDomainWhitelist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hasValue && s.now().Sub(s.lastChecked) < time.Minute {
		return s.value, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qingyunAccountDomainWhitelistURL, nil)
	if err != nil {
		return QingyunAccountDomainWhitelist{}, err
	}
	req.Header.Set("Accept", "application/json")
	if s.etag != "" {
		req.Header.Set("If-None-Match", s.etag)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if s.hasValue {
			s.lastChecked = s.now()
			return s.value, nil
		}
		return QingyunAccountDomainWhitelist{}, fmt.Errorf("load Qingyun account domain whitelist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && s.hasValue {
		s.lastChecked = s.now()
		return s.value, nil
	}
	if resp.StatusCode != http.StatusOK {
		if s.hasValue {
			s.lastChecked = s.now()
			return s.value, nil
		}
		return QingyunAccountDomainWhitelist{}, fmt.Errorf("load Qingyun account domain whitelist: unexpected status %s", resp.Status)
	}

	var value QingyunAccountDomainWhitelist
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&value); err != nil {
		if s.hasValue {
			s.lastChecked = s.now()
			return s.value, nil
		}
		return QingyunAccountDomainWhitelist{}, fmt.Errorf("decode Qingyun account domain whitelist: %w", err)
	}
	if err := validateQingyunAccountDomainWhitelist(value); err != nil {
		if s.hasValue {
			s.lastChecked = s.now()
			return s.value, nil
		}
		return QingyunAccountDomainWhitelist{}, err
	}
	s.value = value
	s.etag = resp.Header.Get("ETag")
	s.lastChecked = s.now()
	s.hasValue = true
	return value, nil
}

func (s *QingyunAccountDomainWhitelistService) ValidateEndpoint(ctx context.Context, endpoint string) error {
	value, err := s.Get(ctx)
	if err != nil {
		return infraerrors.New(http.StatusServiceUnavailable, "QINGYUN_ACCOUNT_WHITELIST_UNAVAILABLE", "账号接口白名单暂时不可用，请稍后重试")
	}
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return infraerrors.BadRequest("QINGYUN_ACCOUNT_ENDPOINT_INVALID", "账号接口地址必须是有效的 HTTP 或 HTTPS URL")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	for _, ip := range value.IPs {
		if host == ip {
			return nil
		}
	}
	for _, pattern := range value.Domains {
		if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(host, strings.TrimPrefix(pattern, "*")) {
			return nil
		}
	}
	return infraerrors.Newf(http.StatusBadRequest, "QINGYUN_ACCOUNT_ENDPOINT_NOT_ALLOWED", "账号接口地址 %q 不在当前白名单中", endpoint)
}

func validateQingyunAccountDomainWhitelist(value QingyunAccountDomainWhitelist) error {
	if value.Version < 1 || (len(value.Domains) == 0 && len(value.IPs) == 0) {
		return fmt.Errorf("invalid Qingyun account domain whitelist")
	}
	for _, domain := range value.Domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if !strings.HasPrefix(domain, "*.") || len(domain) < 4 || strings.ContainsAny(domain[2:], "/:@") {
			return fmt.Errorf("invalid Qingyun account whitelist domain %q", domain)
		}
	}
	for _, ip := range value.IPs {
		if net.ParseIP(strings.TrimSpace(ip)) == nil {
			return fmt.Errorf("invalid Qingyun account whitelist IP %q", ip)
		}
	}
	return nil
}

func (s *adminServiceImpl) validateQingyunAccountEndpoints(ctx context.Context, accountType string, credentials, extra map[string]any) error {
	if s.qingyunDomainWhitelist == nil {
		return nil
	}
	if accountType == AccountTypeAPIKey || accountType == AccountTypeUpstream {
		if endpoint, ok := credentials["base_url"].(string); ok && strings.TrimSpace(endpoint) != "" {
			if err := s.qingyunDomainWhitelist.ValidateEndpoint(ctx, endpoint); err != nil {
				return err
			}
		}
	}
	if endpoint, ok := extra["custom_base_url"].(string); ok && strings.TrimSpace(endpoint) != "" {
		return s.qingyunDomainWhitelist.ValidateEndpoint(ctx, endpoint)
	}
	return nil
}
