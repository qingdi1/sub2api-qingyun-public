package service

import (
	"context"
	"testing"
	"time"
)

func TestQingyunAccountDomainWhitelistValidateEndpoint(t *testing.T) {
	svc := NewQingyunAccountDomainWhitelistService()
	svc.value = QingyunAccountDomainWhitelist{
		Version: 3,
		Domains: []string{"*.qinggekeji.top"},
		IPs:     []string{"47.107.127.143"},
	}
	svc.hasValue = true
	svc.lastChecked = time.Now()

	for _, endpoint := range []string{
		"https://api.qinggekeji.top/v1",
		"http://api.qinggekeji.top/v1",
		"http://47.107.127.143:8888/v1",
	} {
		if err := svc.ValidateEndpoint(context.Background(), endpoint); err != nil {
			t.Fatalf("ValidateEndpoint(%q): %v", endpoint, err)
		}
	}
	if err := svc.ValidateEndpoint(context.Background(), "https://untrusted.example/v1"); err == nil {
		t.Fatal("expected untrusted endpoint to be rejected")
	}
}

func TestQingyunAccountDomainWhitelistRejectsInvalidURL(t *testing.T) {
	svc := NewQingyunAccountDomainWhitelistService()
	svc.value = QingyunAccountDomainWhitelist{Version: 3, Domains: []string{"*.qinggekeji.top"}}
	svc.hasValue = true
	svc.lastChecked = time.Now()
	if err := svc.ValidateEndpoint(context.Background(), "ftp://api.qinggekeji.top/v1"); err == nil {
		t.Fatal("expected non HTTP(S) endpoint to be rejected")
	}
}
