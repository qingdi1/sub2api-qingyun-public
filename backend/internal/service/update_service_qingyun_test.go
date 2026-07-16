package service

import "testing"

func TestQingyunUpdateRepoConstant(t *testing.T) {
	if githubRepo != "qingdi1/sub2api-qingyun" {
		t.Fatalf("githubRepo = %q, want qingdi1/sub2api-qingyun", githubRepo)
	}
}

func TestValidateDownloadURLAllowsGHCR(t *testing.T) {
	cases := []string{
		"https://ghcr.io/qingdi1/sub2api-qingyun:0.1.158",
		"https://pkg-containers.githubusercontent.com/ghcr1/blobs/sha256:abc",
		"https://github.com/qingdi1/sub2api-qingyun/releases/download/v0.1.158/linux_amd64.tar.gz",
	}
	for _, raw := range cases {
		if err := validateDownloadURL(raw); err != nil {
			t.Fatalf("validateDownloadURL(%q) unexpected error: %v", raw, err)
		}
	}
}
