package internal

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfineSavePath(t *testing.T) {
	base := t.TempDir()
	dl := filepath.Join(base, "downloads")
	evil := dl + "-evil"
	outside := filepath.Join(base, "outside")
	for _, d := range []string{dl, evil, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(dl, "link")); err != nil {
		t.Fatal(err)
	}
	roots := []string{dl}
	for name, p := range map[string]string{
		"outside":        outside,
		"etc":            "/etc",
		"sibling prefix": evil,
		"dotdot":         dl + "/../outside",
		"rel dotdot":     "../outside",
		"symlink":        filepath.Join(dl, "link", "x"),
	} {
		if got, err := confineSavePath(p, roots); err == nil {
			t.Errorf("%s: %q accepted as %q", name, p, got)
		}
	}
	if got, err := confineSavePath("", roots); err != nil || got != "" {
		t.Errorf("empty: %q %v", got, err)
	}
	if _, err := confineSavePath(filepath.Join(dl, "tv"), roots); err != nil {
		t.Errorf("inside rejected: %v", err)
	}
	if _, err := confineSavePath("tv/show", roots); err != nil {
		t.Errorf("relative rejected: %v", err)
	}
	if _, err := confineSavePath("/downloads", nil); err == nil {
		t.Error("no roots must fail closed")
	}
}

type fakeRes map[string][]string

func (f fakeRes) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range f[host] {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func TestValidateTorrentURL(t *testing.T) {
	old := torrentResolver
	torrentResolver = fakeRes{
		"tracker.example.com": {"93.184.215.14"},
		"rebind.example.com":  {"10.0.0.7"},
		"meta.example.com":    {"169.254.169.254"},
	}
	t.Cleanup(func() { torrentResolver = old })
	t.Setenv(envIndexerHosts, "")
	ctx := context.Background()

	for _, ok := range []string{
		"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
		"https://tracker.example.com/x.torrent",
	} {
		if err := validateTorrentURL(ctx, ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://127.0.0.1:8080/x",
		"http://localhost/x",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/x.torrent",
		"http://[::1]/x",
		"http://rebind.example.com/x.torrent", // DNS to private
		"http://meta.example.com/x.torrent",   // DNS to metadata
		"http://prowlarr:9696/1/download",     // intranet name
		"file:///etc/passwd",
		"ftp://tracker.example.com/x",
		"https://tracker.example.com/a\nhttps://127.0.0.1/b", // multi-URL injection
	} {
		if err := validateTorrentURL(ctx, bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}

	t.Setenv(envIndexerHosts, "prowlarr:9696")
	if err := validateTorrentURL(ctx, "http://prowlarr:9696/1/download?x=1"); err != nil {
		t.Errorf("allow-listed indexer: %v", err)
	}
	if err := validateTorrentURL(ctx, "http://other:9696/x"); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Errorf("non-listed host must be rejected: %v", err)
	}
}
