package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

type Client struct {
	baseURL  string
	username string
	password string
	client   *http.Client
	cookie   string
}

func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Login(ctx context.Context) error {
	data := url.Values{
		"username": {c.username},
		"password": {c.password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/auth/login", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.baseURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(body))
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" {
			c.cookie = cookie.Value
		}
	}
	if c.cookie == "" {
		return fmt.Errorf("no SID cookie in login response")
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", c.baseURL)
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: "SID", Value: c.cookie})
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return c.client.Do(req)
}

func (c *Client) Add(ctx context.Context, task contracts.DownloadTask) (string, error) {
	data := url.Values{}
	if task.MagnetURI != "" {
		data.Set("urls", task.MagnetURI)
	} else if task.TorrentURL != "" {
		data.Set("urls", task.TorrentURL)
	}
	if task.Label != "" {
		data.Set("category", task.Label)
	}
	if task.DestPath != "" {
		data.Set("savepath", task.DestPath)
	}

	resp, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/add", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("add torrent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("add torrent failed (status %d): %s", resp.StatusCode, string(body))
	}

	// qBittorrent returns "Ok." on success — we need to find the hash from the list
	infos, err := c.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list after add: %w", err)
	}
	if len(infos) > 0 {
		return infos[len(infos)-1].ID, nil
	}
	return "", fmt.Errorf("torrent added but could not determine hash")
}

func (c *Client) Remove(ctx context.Context, id string, deleteData bool) error {
	data := url.Values{
		"hashes":     {id},
		"deleteFiles": {fmt.Sprintf("%v", deleteData)},
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/delete", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("remove torrent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove failed: %s", string(body))
	}
	return nil
}

func (c *Client) Pause(ctx context.Context, id string) error {
	data := url.Values{"hashes": {id}}
	resp, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/pause", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("pause: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) Resume(ctx context.Context, id string) error {
	data := url.Values{"hashes": {id}}
	resp, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/resume", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

type qbtInfo struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	Size        int64   `json:"size"`
	Downloaded  int64   `json:"downloaded"`
	Uploaded    int64   `json:"uploaded"`
	Dlspeed     int64   `json:"dlspeed"`
	Upspeed     int64   `json:"upspeed"`
	ETA         int64   `json:"eta"`
	NumComplete int     `json:"num_complete"`
	NumLeechs   int     `json:"num_leechs"`
	Category    string  `json:"category"`
	SavePath    string  `json:"save_path"`
}

func (c *Client) List(ctx context.Context) ([]contracts.DownloadInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/info", nil)
	if err != nil {
		return nil, fmt.Errorf("list torrents: %w", err)
	}
	defer resp.Body.Close()

	var infos []qbtInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		return nil, fmt.Errorf("decode torrent list: %w", err)
	}

	result := make([]contracts.DownloadInfo, len(infos))
	for i, info := range infos {
		result[i] = contracts.DownloadInfo{
			ID:         info.Hash,
			Name:       info.Name,
			Status:     mapQBState(info.State),
			Progress:   info.Progress,
			SizeBytes:  info.Size,
			Downloaded: info.Downloaded,
			Uploaded:   info.Uploaded,
			SpeedDown:  info.Dlspeed,
			SpeedUp:    info.Upspeed,
			ETA:        info.ETA,
			Seeders:    info.NumComplete,
			Leechers:   info.NumLeechs,
		}
	}
	return result, nil
}

type qbtFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (c *Client) Files(ctx context.Context, hash string) ([]contracts.DownloadFileInfo, error) {
	data := url.Values{"hash": {hash}}
	resp, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/files?"+data.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer resp.Body.Close()

	var files []qbtFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode files: %w", err)
	}

	result := make([]contracts.DownloadFileInfo, len(files))
	for i, f := range files {
		result[i] = contracts.DownloadFileInfo{
			Name: f.Name,
			Size: f.Size,
		}
	}
	return result, nil
}

func (c *Client) Status(ctx context.Context, id string) (contracts.DownloadInfo, error) {
	infos, err := c.List(ctx)
	if err != nil {
		return contracts.DownloadInfo{}, err
	}
	for _, info := range infos {
		if strings.EqualFold(info.ID, id) {
			files, _ := c.Files(ctx, id)
			info.Files = files
			return info, nil
		}
	}
	return contracts.DownloadInfo{}, fmt.Errorf("torrent %q not found", id)
}

func mapQBState(state string) contracts.DownloadStatus {
	switch state {
	case "queuedDL", "queuedUP":
		return contracts.DownloadQueued
	case "downloading", "stalledDL", "metaDL":
		return contracts.DownloadDownloading
	case "uploading", "stalledUP", "forcedUP":
		return contracts.DownloadCompleted
	case "pausedDL", "pausedUP":
		return contracts.DownloadPaused
	case "missingFiles", "error":
		return contracts.DownloadFailed
	default:
		return contracts.DownloadQueued
	}
}
