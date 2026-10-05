package internal

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/sdk/go/module/netguard"
	"github.com/Muxcore-Media/core/sdk/go/module/pathguard"
)

// Environment knobs.
//
//   - QBIT_DOWNLOAD_ROOTS (fallback DOWNLOAD_DIR): comma-separated absolute
//     directories under which AddTorrent may place downloads (RULE-VAL-1).
//     qBittorrent runs elsewhere, so these are the paths as qBittorrent sees
//     them. A non-empty save_path with no roots configured is refused.
//   - DOWNLOADER_INDEXER_HOSTS: comma-separated host[:port] of trusted LAN
//     indexer proxies (e.g. Prowlarr) whose torrent links may resolve to
//     private addresses (RULE-VAL-2).
const (
	envDownloadRoots = "QBIT_DOWNLOAD_ROOTS"
	envDownloadDir   = "DOWNLOAD_DIR"
	envIndexerHosts  = "DOWNLOADER_INDEXER_HOSTS"
)

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func downloadRootsFromEnv() []string {
	if v := splitList(os.Getenv(envDownloadRoots)); len(v) > 0 {
		return v
	}
	return splitList(os.Getenv(envDownloadDir))
}

// confineSavePath validates the caller's save_path against the module's
// download roots. Empty save_path (qBittorrent's own default) is always fine.
func confineSavePath(savePath string, roots []string) (string, error) {
	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		return "", nil
	}
	if len(roots) == 0 {
		return "", fmt.Errorf("save_path refused: no download roots configured (set %s)", envDownloadRoots)
	}
	clean := make([]string, 0, len(roots))
	for _, r := range roots {
		clean = append(clean, filepath.Clean(r))
	}
	if !filepath.IsAbs(savePath) {
		// Relative paths are taken under the first root.
		return pathguard.Join(clean[0], savePath)
	}
	if _, err := pathguard.Confine(savePath, clean); err != nil {
		return "", fmt.Errorf("save_path rejected: %w", err)
	}
	return filepath.Clean(savePath), nil
}

// torrentResolver is the DNS hook (tests replace it).
var torrentResolver netguard.Resolver

func torrentNetOptions() netguard.Options {
	return netguard.Options{Timeout: 10 * time.Second, Resolver: torrentResolver}
}

// validateTorrentURL guards the URL handed to qBittorrent's "urls" parameter.
// qBittorrent fetches http(s) torrent links itself, so the dial cannot be
// guarded here; instead the URL is validated and its host name resolved and
// checked. Magnet links are not fetched and pass through. A newline would make
// qBittorrent treat the value as several URLs and is refused.
func validateTorrentURL(ctx context.Context, raw string) error {
	if strings.ContainsAny(raw, "\r\n\x00") {
		return fmt.Errorf("torrent_url must be a single URL")
	}
	t := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(t), "magnet:") {
		return nil
	}
	if t != raw {
		return fmt.Errorf("torrent_url has surrounding whitespace")
	}
	opts := torrentNetOptions()
	profile := netguard.UserURL
	if hosts := splitList(os.Getenv(envIndexerHosts)); len(hosts) > 0 {
		trusted := netguard.Options{AllowPrivate: true, AllowLoopback: true, AllowedHosts: hosts, Resolver: torrentResolver}
		if netguard.ValidateURL(raw, netguard.Integration, trusted) == nil {
			return nil // listed LAN indexer proxy: operator-approved
		}
	}
	if err := netguard.ValidateURL(raw, profile, opts); err != nil {
		return fmt.Errorf("torrent_url rejected: %w", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("torrent_url rejected: %w", err)
	}
	host := u.Hostname()
	if _, err := netip.ParseAddr(host); err == nil {
		return nil // literal already checked by ValidateURL
	}
	res := torrentResolver
	if res == nil {
		res = defaultResolver{}
	}
	rctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	addrs, err := res.LookupNetIP(rctx, "ip", host)
	if err != nil {
		return fmt.Errorf("torrent_url rejected: resolve %q: %w", host, err)
	}
	for _, a := range addrs {
		if err := netguard.CheckAddr(a, profile, opts); err != nil {
			return fmt.Errorf("torrent_url rejected: %w", err)
		}
	}
	return nil
}
