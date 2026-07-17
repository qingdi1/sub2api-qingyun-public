package service

import "testing"

func TestQingyunUpdateRepoConstant(t *testing.T) {
	if githubRepo != "qingdi1/sub2api-qingyun-public" {
		t.Fatalf("githubRepo = %q, want qingdi1/sub2api-qingyun-public", githubRepo)
	}
}

func TestValidateDownloadURLAllowsGHCR(t *testing.T) {
	cases := []string{
		"https://ghcr.io/qingdi1/sub2api-qingyun-public:0.1.158",
		"https://pkg-containers.githubusercontent.com/ghcr1/blobs/sha256:abc",
		"https://github.com/qingdi1/sub2api-qingyun-public/releases/download/v0.1.158/linux_amd64.tar.gz",
	}
	for _, raw := range cases {
		if err := validateDownloadURL(raw); err != nil {
			t.Fatalf("validateDownloadURL(%q) unexpected error: %v", raw, err)
		}
	}
}

func TestCompareVersionsHandlesQingyunSuffix(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{name: "older upstream is not an update", current: "0.1.158-qingyun.1", latest: "0.1.151", want: 1},
		{name: "same upstream core is current", current: "0.1.158-qingyun.1", latest: "v0.1.158", want: 0},
		{name: "newer upstream is an update", current: "0.1.158-qingyun.1", latest: "0.1.159", want: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := compareVersions(test.current, test.latest); got != test.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.current, test.latest, got, test.want)
			}
		})
	}
}
