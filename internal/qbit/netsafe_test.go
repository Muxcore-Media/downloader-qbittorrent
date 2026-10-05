package qbit

import (
	"context"
	"strings"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1:8080", "http://qbittorrent:8080", "http://192.168.1.10:8080", "https://qbit.lan:8443/"} {
		if err := ValidateBaseURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://169.254.169.254/", "http://metadata.google.internal/", "file:///etc/passwd", "ftp://x/", "http://0.0.0.0:8080", "http://[fe80::1]/", ""} {
		if err := ValidateBaseURL(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestClientRefusesMetadataBaseURL(t *testing.T) {
	c := &Client{BaseURL: "http://169.254.169.254"}
	_, err := c.GetTorrent(context.Background(), "abc")
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected rejection, got %v", err)
	}
}
