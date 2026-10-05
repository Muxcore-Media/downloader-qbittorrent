package qbit

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FixtureClient is an in-memory Downloader backend for CI / DOWNLOADER_ENGINE=fixture.
// No HTTP and no live qBittorrent required.
type FixtureClient struct {
	mu            sync.Mutex
	torrents      map[string]*Torrent
	magnets       map[string]string
	categories    map[string]string
	downloadLimit int64
	uploadLimit   int64
}

// NewFixtureClient returns an empty fixture store.
func NewFixtureClient() *FixtureClient {
	return &FixtureClient{
		torrents:   map[string]*Torrent{},
		magnets:    map[string]string{},
		categories: map[string]string{},
	}
}

func (f *FixtureClient) AddTorrent(_ context.Context, torrentURL, savePath, category string, paused bool) (string, *Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if torrentURL == "" {
		return "", nil, fmt.Errorf("torrent_url required")
	}
	hash := ResolveHash(torrentURL)
	if hash == "" {
		sum := sha1.Sum([]byte(torrentURL))
		hash = hex.EncodeToString(sum[:])
	}
	name := torrentDisplayName(torrentURL, savePath)
	if savePath == "" {
		savePath = "/downloads/fixture"
	}
	state := "downloading"
	if paused {
		state = "pausedDL"
	}
	t := &Torrent{
		Hash: hash, Name: name, Size: 8192, Downloaded: 4096,
		Progress: 0.5, Dlspeed: 1024, State: state, SavePath: savePath,
		Category: category, AddedOn: time.Now().Unix(),
	}
	// Fixture completes instantly unless paused (matches native fixture behavior).
	if !paused {
		t.State = "uploading"
		t.Progress = 1
		t.Downloaded = t.Size
		t.CompletionOn = time.Now().Unix()
		if rel, err := materializeFixtureMedia(savePath, name, t.Size); err == nil {
			t.ContentPath = filepath.Join(savePath, filepath.FromSlash(rel))
			if info, statErr := os.Stat(t.ContentPath); statErr == nil {
				t.Size = info.Size()
				t.Downloaded = info.Size()
			}
		}
	}
	f.torrents[hash] = t
	f.magnets[hash] = torrentURL
	return hash, t, nil
}

// materializeFixtureMedia writes a video file under savePath for scanner import smoke tests.
// When FIXTURE_MEDIA_SEED (or QBIT_FIXTURE_MEDIA_SEED) points at a real file, that file is copied.
// The seed content is always the same bytes regardless of title — use only with fixture indexers.
func materializeFixtureMedia(savePath, name string, size int64) (rel string, err error) {
	if savePath == "" {
		return "", fmt.Errorf("save_path required")
	}
	if name == "" {
		name = "Fixture.Movie.1999.1080p.BluRay"
	}
	rel = filepath.ToSlash(filepath.Join(name, name+".mkv"))
	abs := filepath.Join(savePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if seed := fixtureMediaSeed(); seed != "" {
		if err := copyFixtureSeed(seed, abs); err != nil {
			return "", err
		}
		return rel, nil
	}
	if size < 1 {
		size = 8192
	}
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	if err := os.WriteFile(abs, payload, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

func fixtureMediaSeed() string {
	if v := strings.TrimSpace(os.Getenv("QBIT_FIXTURE_MEDIA_SEED")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("FIXTURE_MEDIA_SEED"))
}

func copyFixtureSeed(seed, dest string) error {
	src, err := os.Open(seed)
	if err != nil {
		return fmt.Errorf("open fixture seed %q: %w", seed, err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}

func torrentDisplayName(torrentURL, savePath string) string {
	name := "fixture"
	if i := strings.Index(torrentURL, "dn="); i >= 0 {
		rest := torrentURL[i+3:]
		if j := strings.IndexAny(rest, "&"); j >= 0 {
			rest = rest[:j]
		}
		if rest != "" {
			name = rest
		}
	}
	if name != "fixture" {
		return name
	}
	// Torznab/http download URLs have no dn=; derive a stable folder name from the partial path.
	clean := filepath.Clean(strings.TrimSpace(savePath))
	base := filepath.Base(clean)
	if strings.HasPrefix(base, "pending_") {
		parent := filepath.Base(filepath.Dir(clean))
		if parent != "" && parent != "." && parent != "partials" && !strings.HasPrefix(parent, "pending_") {
			return parent
		}
	}
	return name
}

func (f *FixtureClient) List(_ context.Context, category, status string) ([]Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Torrent, 0, len(f.torrents))
	for _, t := range f.torrents {
		if category != "" && t.Category != category {
			continue
		}
		if status != "" && MapState(t.State) != status && t.State != status {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
}

func (f *FixtureClient) Get(_ context.Context, hash string) (*Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[strings.ToLower(hash)]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (f *FixtureClient) Pause(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[strings.ToLower(hash)]
	if !ok {
		return fmt.Errorf("torrent %q not found", hash)
	}
	t.State = "pausedDL"
	return nil
}

func (f *FixtureClient) Resume(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[strings.ToLower(hash)]
	if !ok {
		return fmt.Errorf("torrent %q not found", hash)
	}
	t.State = "uploading"
	t.Progress = 1
	t.Downloaded = t.Size
	t.CompletionOn = time.Now().Unix()
	if rel, err := materializeFixtureMedia(t.SavePath, t.Name, t.Size); err == nil {
		t.ContentPath = filepath.Join(t.SavePath, filepath.FromSlash(rel))
		if info, statErr := os.Stat(t.ContentPath); statErr == nil {
			t.Size = info.Size()
			t.Downloaded = info.Size()
		}
	}
	return nil
}

func (f *FixtureClient) Delete(_ context.Context, hash string, deleteFiles bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := strings.ToLower(hash)
	t, ok := f.torrents[h]
	if !ok {
		return fmt.Errorf("torrent %q not found", hash)
	}
	if deleteFiles {
		if t.ContentPath != "" {
			_ = os.RemoveAll(filepath.Dir(t.ContentPath))
		} else if t.SavePath != "" {
			_ = os.RemoveAll(filepath.Join(t.SavePath, t.Name))
		}
	}
	delete(f.torrents, h)
	delete(f.magnets, h)
	return nil
}

func (f *FixtureClient) CreateCategory(_ context.Context, name, savePath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "" {
		return fmt.Errorf("category name required")
	}
	f.categories[name] = savePath
	return nil
}

func (f *FixtureClient) DeleteCategory(_ context.Context, names string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, name := range strings.Split(names, "|") {
		name = strings.TrimSpace(name)
		if name != "" {
			delete(f.categories, name)
		}
	}
	return nil
}

func (f *FixtureClient) RenameCategory(_ context.Context, oldName, newName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if oldName == "" || newName == "" {
		return fmt.Errorf("category names required")
	}
	savePath, ok := f.categories[oldName]
	if !ok {
		return fmt.Errorf("category %q not found", oldName)
	}
	delete(f.categories, oldName)
	f.categories[newName] = savePath
	for _, t := range f.torrents {
		if t.Category == oldName {
			t.Category = newName
		}
	}
	return nil
}

func (f *FixtureClient) GetCategories(_ context.Context) (map[string]Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]Category, len(f.categories))
	for name, savePath := range f.categories {
		out[name] = Category{Name: name, SavePath: savePath}
	}
	return out, nil
}

func (f *FixtureClient) SetDownloadLimit(_ context.Context, limitBytesPerSec int64) error {
	f.mu.Lock()
	f.downloadLimit = limitBytesPerSec
	f.mu.Unlock()
	return nil
}

func (f *FixtureClient) SetUploadLimit(_ context.Context, limitBytesPerSec int64) error {
	f.mu.Lock()
	f.uploadLimit = limitBytesPerSec
	f.mu.Unlock()
	return nil
}

func (f *FixtureClient) GetTransferInfo(_ context.Context) (*TransferInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var dlSpeed, upSpeed int64
	for _, t := range f.torrents {
		dlSpeed += t.Dlspeed
		upSpeed += t.Upspeed
	}
	return &TransferInfo{
		DownloadSpeed:    dlSpeed,
		UploadSpeed:      upSpeed,
		DownloadLimit:    f.downloadLimit,
		UploadLimit:      f.uploadLimit,
		ConnectionStatus: "connected",
	}, nil
}

func (f *FixtureClient) Recheck(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[strings.ToLower(hash)]
	if !ok {
		return fmt.Errorf("torrent %q not found", hash)
	}
	t.State = "checkingDL"
	t.LastActivity = time.Now().Unix()
	return nil
}

func (f *FixtureClient) StuckDownloads(_ context.Context) ([]Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().Unix()
	cutoff := int64(stuckDownloadThreshold.Seconds())
	var stuck []Torrent
	for _, t := range f.torrents {
		state := strings.ToLower(t.State)
		if state != "stalleddl" && state != "error" && state != "missingfiles" {
			continue
		}
		last := t.LastActivity
		if last == 0 {
			last = t.AddedOn
		}
		if last > 0 && now-last >= cutoff {
			stuck = append(stuck, *t)
		}
	}
	return stuck, nil
}

func (f *FixtureClient) ReAddTorrent(_ context.Context, magnetURL, savePath, category string) error {
	hash := ResolveHash(magnetURL)
	if hash == "" {
		return fmt.Errorf("magnet hash required")
	}
	hash = strings.ToLower(hash)
	f.mu.Lock()
	if savePath == "" || category == "" {
		if t, ok := f.torrents[hash]; ok {
			if savePath == "" {
				savePath = t.SavePath
			}
			if category == "" {
				category = t.Category
			}
		}
	}
	delete(f.torrents, hash)
	f.mu.Unlock()
	_, _, err := f.AddTorrent(context.Background(), magnetURL, savePath, category, false)
	if err != nil {
		return err
	}
	return f.Recheck(context.Background(), hash)
}

// MarkStuck sets a torrent into a stuck state for fixture testing.
func (f *FixtureClient) MarkStuck(hash string, state string, lastActivity time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[strings.ToLower(hash)]
	if !ok {
		return fmt.Errorf("torrent %q not found", hash)
	}
	if state == "" {
		state = "stalledDL"
	}
	t.State = state
	t.LastActivity = lastActivity.Unix()
	return nil
}
