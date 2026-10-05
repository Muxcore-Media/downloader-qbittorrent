// Package qbit implements a minimal qBittorrent WebUI API client.
package qbit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client talks to qBittorrent's /api/v2 endpoints.
type Client struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client

	mu     sync.Mutex
	logged bool
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		if c.HTTPClient.Jar == nil {
			jar, _ := cookiejar.New(nil)
			c.HTTPClient = &http.Client{
				Transport:     c.HTTPClient.Transport,
				CheckRedirect: c.HTTPClient.CheckRedirect,
				Timeout:       c.HTTPClient.Timeout,
				Jar:           jar,
			}
		}
		return c.HTTPClient
	}
	c.HTTPClient = newGuardedHTTPClient()
	return c.HTTPClient
}

func (c *Client) apiURL(path string) (string, error) {
	if c.BaseURL == "" {
		return "", fmt.Errorf("qbittorrent base URL required")
	}
	if err := ValidateBaseURL(c.BaseURL); err != nil {
		return "", fmt.Errorf("qbittorrent base URL rejected: %w", err)
	}
	base := strings.TrimRight(c.BaseURL, "/")
	return base + "/api/v2" + path, nil
}

func (c *Client) setCSRFHeaders(req *http.Request) {
	base := strings.TrimRight(c.BaseURL, "/")
	req.Header.Set("Referer", base+"/")
	req.Header.Set("Origin", base)
}

func (c *Client) ensureLogin(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.logged {
		return nil
	}
	return c.loginLocked(ctx)
}

func (c *Client) loginLocked(ctx context.Context) error {
	u, err := c.apiURL("/auth/login")
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("username", c.Username)
	form.Set("password", c.Password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.setCSRFHeaders(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("qbittorrent login: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if s := strings.TrimSpace(string(body)); s != "" && !strings.EqualFold(s, "Ok.") {
		return fmt.Errorf("qbittorrent login failed: %s", s)
	}
	c.logged = true
	return nil
}

func (c *Client) clearLogin() {
	c.mu.Lock()
	c.logged = false
	c.mu.Unlock()
}

func (c *Client) doForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	return c.doRequest(ctx, http.MethodPost, path, form, nil)
}

func (c *Client) doGET(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.doRequest(ctx, http.MethodGet, path, nil, query)
}

func (c *Client) doRequest(ctx context.Context, method, path string, form url.Values, query url.Values) ([]byte, error) {
	body, err := c.doRequestOnce(ctx, method, path, form, query, false)
	if err == nil {
		return body, nil
	}
	if !strings.Contains(err.Error(), "forbidden") {
		return nil, err
	}
	c.clearLogin()
	return c.doRequestOnce(ctx, method, path, form, query, true)
}

func (c *Client) doRequestOnce(ctx context.Context, method, path string, form url.Values, query url.Values, retried bool) ([]byte, error) {
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
	}
	u, err := c.apiURL(path)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var bodyReader io.Reader
	if form != nil {
		bodyReader = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	c.setCSRFHeaders(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		c.clearLogin()
		msg := fmt.Errorf("qbittorrent %s: forbidden (re-login required)", path)
		if !retried {
			return nil, msg
		}
		return nil, msg
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qbittorrent %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// TorrentFileInfo is a row from /torrents/files.
type TorrentFileInfo struct {
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
}

// Torrent is a simplified torrents/info row.
type Torrent struct {
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	Size         int64   `json:"size"`
	Downloaded   int64   `json:"downloaded"`
	Uploaded     int64   `json:"uploaded"`
	Progress     float64 `json:"progress"`
	Dlspeed      int64   `json:"dlspeed"`
	Upspeed      int64   `json:"upspeed"`
	NumSeeds     int     `json:"num_seeds"`
	NumLeechs    int     `json:"num_leechs"`
	State        string  `json:"state"`
	SavePath     string  `json:"save_path"`
	ContentPath  string  `json:"content_path,omitempty"`
	Category     string  `json:"category"`
	AddedOn      int64   `json:"added_on"`
	CompletionOn int64   `json:"completion_on"`
	LastActivity int64   `json:"last_activity"`
	Files        []TorrentFileInfo
}

// Category describes a qBittorrent download category.
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// TransferInfo is the global transfer state from /transfer/info.
type TransferInfo struct {
	DownloadSpeed     int64  `json:"dl_info_speed"`
	UploadSpeed       int64  `json:"up_info_speed"`
	DownloadLimit     int64  `json:"dl_rate_limit"`
	UploadLimit       int64  `json:"up_rate_limit"`
	DownloadedSession int64  `json:"dl_info_data"`
	UploadedSession   int64  `json:"up_info_data"`
	DownloadedAllTime int64  `json:"alltime_dl"`
	UploadedAllTime   int64  `json:"alltime_ul"`
	ConnectionStatus  string `json:"connection_status"`
	DHTNodes          int    `json:"dht_nodes"`
	FreeSpaceOnDisk   int64  `json:"free_space_on_disk"`
}

const stuckDownloadThreshold = 15 * time.Minute

// AddTorrent queues a magnet or .torrent URL.
func (c *Client) AddTorrent(ctx context.Context, torrentURL, savePath, category string, paused bool) error {
	form := url.Values{}
	form.Set("urls", torrentURL)
	if savePath != "" {
		form.Set("savepath", savePath)
	}
	if category != "" {
		form.Set("category", category)
	}
	if paused {
		form.Set("paused", "true")
		form.Set("stopped", "true")
	}
	_, err := c.doForm(ctx, "/torrents/add", form)
	return err
}

// ResolveHash returns the expected info-hash for a magnet URL, or empty for non-magnets.
func ResolveHash(torrentURL string) string {
	return ParseMagnetHash(torrentURL)
}

// ListTorrents returns torrents, optionally filtered by category.
func (c *Client) ListTorrents(ctx context.Context, category, filter string) ([]Torrent, error) {
	q := url.Values{}
	if category != "" {
		q.Set("category", category)
	}
	if filter != "" {
		q.Set("filter", filter)
	}
	body, err := c.doGET(ctx, "/torrents/info", q)
	if err != nil {
		return nil, err
	}
	var out []Torrent
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode torrents/info: %w", err)
	}
	return out, nil
}

// GetTorrent returns one torrent by hash, or nil if missing.
func (c *Client) GetTorrent(ctx context.Context, hash string) (*Torrent, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return nil, fmt.Errorf("hash required")
	}
	q := url.Values{"hashes": {hash}}
	body, err := c.doGET(ctx, "/torrents/info", q)
	if err != nil {
		return nil, err
	}
	var list []Torrent
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("decode torrents/info: %w", err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	t := list[0]
	files, err := c.TorrentFiles(ctx, hash)
	if err == nil {
		t.Files = files
	}
	return &t, nil
}

// TorrentFiles lists files for one torrent hash.
func (c *Client) TorrentFiles(ctx context.Context, hash string) ([]TorrentFileInfo, error) {
	q := url.Values{"hash": {hash}}
	body, err := c.doGET(ctx, "/torrents/files", q)
	if err != nil {
		return nil, err
	}
	var out []TorrentFileInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode torrents/files: %w", err)
	}
	return out, nil
}

func (c *Client) pausePath() string  { return "/torrents/stop" }
func (c *Client) resumePath() string { return "/torrents/start" }

func (c *Client) pauseFallback() string  { return "/torrents/pause" }
func (c *Client) resumeFallback() string { return "/torrents/resume" }

func (c *Client) formOrFallback(ctx context.Context, primary, fallback string, form url.Values) error {
	_, err := c.doForm(ctx, primary, form)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "HTTP 404") {
		_, err = c.doForm(ctx, fallback, form)
	}
	return err
}

// Pause pauses one or more torrents by hash (pipe-separated).
func (c *Client) Pause(ctx context.Context, hashes string) error {
	form := url.Values{"hashes": {hashes}}
	return c.formOrFallback(ctx, c.pausePath(), c.pauseFallback(), form)
}

// Resume resumes one or more torrents by hash.
func (c *Client) Resume(ctx context.Context, hashes string) error {
	form := url.Values{"hashes": {hashes}}
	return c.formOrFallback(ctx, c.resumePath(), c.resumeFallback(), form)
}

// Delete removes torrents; optionally delete files from disk.
func (c *Client) Delete(ctx context.Context, hashes string, deleteFiles bool) error {
	form := url.Values{"hashes": {hashes}}
	if deleteFiles {
		form.Set("deleteFiles", "true")
	} else {
		form.Set("deleteFiles", "false")
	}
	_, err := c.doForm(ctx, "/torrents/delete", form)
	return err
}

// Version hits /app/version as a cheap health probe.
func (c *Client) Version(ctx context.Context) (string, error) {
	body, err := c.doGET(ctx, "/app/version", nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// CreateCategory registers a new torrent category.
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{"category": {name}}
	if savePath != "" {
		form.Set("savePath", savePath)
	}
	_, err := c.doForm(ctx, "/torrents/createCategory", form)
	return err
}

// DeleteCategory removes one or more categories (pipe-separated names).
func (c *Client) DeleteCategory(ctx context.Context, names string) error {
	form := url.Values{"categories": {names}}
	_, err := c.doForm(ctx, "/torrents/removeCategories", form)
	return err
}

// RenameCategory renames an existing category.
func (c *Client) RenameCategory(ctx context.Context, oldName, newName string) error {
	form := url.Values{"oldCategory": {oldName}, "newCategory": {newName}}
	_, err := c.doForm(ctx, "/torrents/renameCategory", form)
	return err
}

// GetCategories returns all configured categories.
func (c *Client) GetCategories(ctx context.Context) (map[string]Category, error) {
	body, err := c.doGET(ctx, "/torrents/categories", nil)
	if err != nil {
		return nil, err
	}
	var out map[string]Category
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode torrents/categories: %w", err)
	}
	return out, nil
}

// SetDownloadLimit sets the global download speed limit in bytes/s (0 = unlimited).
func (c *Client) SetDownloadLimit(ctx context.Context, limitBytesPerSec int64) error {
	form := url.Values{"limit": {fmt.Sprintf("%d", limitBytesPerSec)}}
	_, err := c.doForm(ctx, "/torrents/setDownloadLimit", form)
	return err
}

// SetUploadLimit sets the global upload speed limit in bytes/s (0 = unlimited).
func (c *Client) SetUploadLimit(ctx context.Context, limitBytesPerSec int64) error {
	form := url.Values{"limit": {fmt.Sprintf("%d", limitBytesPerSec)}}
	_, err := c.doForm(ctx, "/torrents/setUploadLimit", form)
	return err
}

// GetTransferInfo returns global transfer statistics and rate limits.
func (c *Client) GetTransferInfo(ctx context.Context) (*TransferInfo, error) {
	body, err := c.doGET(ctx, "/transfer/info", nil)
	if err != nil {
		return nil, err
	}
	var out TransferInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode transfer/info: %w", err)
	}
	return &out, nil
}

// Recheck forces a hash recheck on one or more torrents (pipe-separated hashes).
func (c *Client) Recheck(ctx context.Context, hashes string) error {
	form := url.Values{"hashes": {hashes}}
	_, err := c.doForm(ctx, "/torrents/recheck", form)
	return err
}

// StuckDownloads returns torrents stalled or errored for longer than 15 minutes.
func (c *Client) StuckDownloads(ctx context.Context) ([]Torrent, error) {
	all, err := c.ListTorrents(ctx, "", "")
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	cutoff := int64(stuckDownloadThreshold.Seconds())
	var stuck []Torrent
	for _, t := range all {
		state := strings.ToLower(t.State)
		if state != "stalleddl" && state != "error" && state != "missingfiles" {
			continue
		}
		last := t.LastActivity
		if last == 0 {
			last = t.AddedOn
		}
		if last > 0 && now-last >= cutoff {
			stuck = append(stuck, t)
		}
	}
	return stuck, nil
}

// ReAddTorrent deletes and re-queues a magnet, then forces a recheck.
func (c *Client) ReAddTorrent(ctx context.Context, magnetURL, savePath, category string) error {
	hash := ResolveHash(magnetURL)
	if hash == "" {
		return fmt.Errorf("magnet hash required")
	}
	if savePath == "" || category == "" {
		existing, err := c.GetTorrent(ctx, hash)
		if err != nil {
			return err
		}
		if existing != nil {
			if savePath == "" {
				savePath = existing.SavePath
			}
			if category == "" {
				category = existing.Category
			}
		}
	}
	if err := c.Delete(ctx, hash, false); err != nil {
		return err
	}
	if err := c.AddTorrent(ctx, magnetURL, savePath, category, false); err != nil {
		return err
	}
	return c.Recheck(ctx, hash)
}

// MapState converts qBit state strings to contracts-downloader status labels.
func MapState(state string) string {
	switch strings.ToLower(state) {
	case "uploading", "stalledup", "forcedup", "queuedup", "pausedup", "checkingup":
		return "completed"
	case "downloading", "forceddl", "metadl", "queueddl", "allocating", "checkingdl", "checkingresumedata", "moving":
		return "downloading"
	case "pauseddl":
		return "paused"
	case "error", "missingfiles":
		return "failed"
	case "stalleddl":
		return "stalled"
	default:
		if state == "" {
			return "unknown"
		}
		return state
	}
}
