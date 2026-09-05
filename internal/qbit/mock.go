package qbit

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MockServer is an in-process qBittorrent WebUI stand-in for offline tests.
type MockServer struct {
	Username string
	Password string
	Server   *httptest.Server

	mu            sync.Mutex
	seq           atomic.Uint64
	torrents      map[string]*Torrent
	files         map[string][]TorrentFileInfo
	authed        map[string]bool
	categories    map[string]string
	downloadLimit int64
	uploadLimit   int64
}

// NewMockServer starts an httptest WebUI that accepts the given credentials.
func NewMockServer(user, pass string) *MockServer {
	if user == "" {
		user = "admin"
	}
	if pass == "" {
		pass = "adminadmin"
	}
	m := &MockServer{
		Username:   user,
		Password:   pass,
		torrents:   map[string]*Torrent{},
		files:      map[string][]TorrentFileInfo{},
		authed:     map[string]bool{},
		categories: map[string]string{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", m.handleLogin)
	mux.HandleFunc("/api/v2/app/version", m.requireAuth(m.handleVersion))
	mux.HandleFunc("/api/v2/torrents/add", m.requireAuth(m.handleAdd))
	mux.HandleFunc("/api/v2/torrents/info", m.requireAuth(m.handleInfo))
	mux.HandleFunc("/api/v2/torrents/files", m.requireAuth(m.handleFiles))
	mux.HandleFunc("/api/v2/torrents/pause", m.requireAuth(m.handlePause))
	mux.HandleFunc("/api/v2/torrents/resume", m.requireAuth(m.handleResume))
	mux.HandleFunc("/api/v2/torrents/stop", m.requireAuth(m.handlePause))
	mux.HandleFunc("/api/v2/torrents/start", m.requireAuth(m.handleResume))
	mux.HandleFunc("/api/v2/torrents/delete", m.requireAuth(m.handleDelete))
	mux.HandleFunc("/api/v2/torrents/createCategory", m.requireAuth(m.handleCreateCategory))
	mux.HandleFunc("/api/v2/torrents/removeCategories", m.requireAuth(m.handleRemoveCategories))
	mux.HandleFunc("/api/v2/torrents/renameCategory", m.requireAuth(m.handleRenameCategory))
	mux.HandleFunc("/api/v2/torrents/categories", m.requireAuth(m.handleCategories))
	mux.HandleFunc("/api/v2/torrents/setDownloadLimit", m.requireAuth(m.handleSetDownloadLimit))
	mux.HandleFunc("/api/v2/torrents/setUploadLimit", m.requireAuth(m.handleSetUploadLimit))
	mux.HandleFunc("/api/v2/transfer/info", m.requireAuth(m.handleTransferInfo))
	mux.HandleFunc("/api/v2/torrents/recheck", m.requireAuth(m.handleRecheck))
	m.Server = httptest.NewServer(mux)
	return m
}

func (m *MockServer) URL() string { return m.Server.URL }

func (m *MockServer) Close() { m.Server.Close() }

func (m *MockServer) Client() *Client {
	return &Client{
		BaseURL:    m.URL(),
		Username:   m.Username,
		Password:   m.Password,
		HTTPClient: m.Server.Client(),
	}
}

func (m *MockServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("SID")
		m.mu.Lock()
		ok := err == nil && m.authed[cookie.Value]
		m.mu.Unlock()
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (m *MockServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.Form.Get("username") != m.Username || r.Form.Get("password") != m.Password {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Fails."))
		return
	}
	sid := fmt.Sprintf("sid-%d", m.seq.Add(1))
	m.mu.Lock()
	m.authed[sid] = true
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "SID", Value: sid, Path: "/"})
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleVersion(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("v4.6.0"))
}

func (m *MockServer) handleAdd(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	urls := r.Form.Get("urls")
	if urls == "" {
		http.Error(w, "empty urls", http.StatusBadRequest)
		return
	}
	hash := hashFromURL(urls)
	name := nameFromURL(urls)
	save := r.Form.Get("savepath")
	if save == "" {
		save = "/downloads"
	}
	state := "downloading"
	if r.Form.Get("paused") == "true" || r.Form.Get("stopped") == "true" {
		state = "pausedDL"
	}
	contentPath := filepath.Join(save, name, name+".mkv")
	t := &Torrent{
		Hash: hash, Name: name, Size: 1024 * 1024, Downloaded: 0,
		Progress: 0.1, Dlspeed: 1024, State: state, SavePath: save,
		ContentPath: contentPath,
		Category:    r.Form.Get("category"), AddedOn: time.Now().Unix(),
		LastActivity: time.Now().Unix(),
	}
	files := []TorrentFileInfo{{Name: name + ".mkv", Size: t.Size, Progress: t.Progress}}
	m.mu.Lock()
	m.torrents[hash] = t
	m.files[hash] = files
	m.mu.Unlock()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	hashes := r.URL.Query().Get("hashes")
	want := map[string]bool{}
	if hashes != "" {
		for _, h := range strings.Split(hashes, "|") {
			want[strings.ToLower(strings.TrimSpace(h))] = true
		}
	}
	m.mu.Lock()
	out := make([]Torrent, 0, len(m.torrents))
	for _, t := range m.torrents {
		if len(want) > 0 && !want[strings.ToLower(t.Hash)] {
			continue
		}
		if cat != "" && t.Category != cat {
			continue
		}
		out = append(out, *t)
	}
	m.mu.Unlock()
	_ = json.NewEncoder(w).Encode(out)
}

func (m *MockServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	hash := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("hash")))
	m.mu.Lock()
	files := m.files[hash]
	m.mu.Unlock()
	if files == nil {
		files = []TorrentFileInfo{}
	}
	_ = json.NewEncoder(w).Encode(files)
}

func (m *MockServer) handlePause(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	m.setState(r.Form.Get("hashes"), "pausedDL")
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleResume(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	m.setState(r.Form.Get("hashes"), "downloading")
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	hashes := strings.Split(r.Form.Get("hashes"), "|")
	m.mu.Lock()
	for _, h := range hashes {
		h = strings.ToLower(strings.TrimSpace(h))
		delete(m.torrents, h)
		delete(m.files, h)
	}
	m.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) setState(hashes, state string) {
	parts := strings.Split(hashes, "|")
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range parts {
		h = strings.ToLower(strings.TrimSpace(h))
		if t, ok := m.torrents[h]; ok {
			t.State = state
		}
	}
}

// Complete marks a torrent finished (uploading).
func (m *MockServer) Complete(hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.torrents[strings.ToLower(hash)]
	if !ok {
		return false
	}
	t.State = "uploading"
	t.Progress = 1
	t.Downloaded = t.Size
	t.CompletionOn = time.Now().Unix()
	if files, ok := m.files[t.Hash]; ok {
		for i := range files {
			files[i].Progress = 1
		}
	}
	return true
}

// Fail marks a torrent errored.
func (m *MockServer) Fail(hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.torrents[strings.ToLower(hash)]
	if !ok {
		return false
	}
	t.State = "error"
	return true
}

func (m *MockServer) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	name := r.Form.Get("category")
	if name == "" {
		http.Error(w, "category required", http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	m.categories[name] = r.Form.Get("savePath")
	m.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleRemoveCategories(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	m.mu.Lock()
	for _, name := range strings.Split(r.Form.Get("categories"), "|") {
		name = strings.TrimSpace(name)
		if name != "" {
			delete(m.categories, name)
		}
	}
	m.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleRenameCategory(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	oldName := r.Form.Get("oldCategory")
	newName := r.Form.Get("newCategory")
	m.mu.Lock()
	savePath, ok := m.categories[oldName]
	if ok {
		delete(m.categories, oldName)
		m.categories[newName] = savePath
	}
	for _, t := range m.torrents {
		if t.Category == oldName {
			t.Category = newName
		}
	}
	m.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleCategories(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	out := make(map[string]Category, len(m.categories))
	for name, savePath := range m.categories {
		out[name] = Category{Name: name, SavePath: savePath}
	}
	m.mu.Unlock()
	_ = json.NewEncoder(w).Encode(out)
}

func (m *MockServer) handleSetDownloadLimit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	limit, _ := strconv.ParseInt(r.Form.Get("limit"), 10, 64)
	m.mu.Lock()
	m.downloadLimit = limit
	m.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleSetUploadLimit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	limit, _ := strconv.ParseInt(r.Form.Get("limit"), 10, 64)
	m.mu.Lock()
	m.uploadLimit = limit
	m.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (m *MockServer) handleTransferInfo(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	var dlSpeed, upSpeed int64
	for _, t := range m.torrents {
		dlSpeed += t.Dlspeed
		upSpeed += t.Upspeed
	}
	info := TransferInfo{
		DownloadSpeed: dlSpeed, UploadSpeed: upSpeed,
		DownloadLimit: m.downloadLimit, UploadLimit: m.uploadLimit,
		ConnectionStatus: "connected",
	}
	m.mu.Unlock()
	_ = json.NewEncoder(w).Encode(info)
}

func (m *MockServer) handleRecheck(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	for _, h := range strings.Split(r.Form.Get("hashes"), "|") {
		h = strings.ToLower(strings.TrimSpace(h))
		m.mu.Lock()
		if t, ok := m.torrents[h]; ok {
			t.State = "checkingDL"
			t.LastActivity = time.Now().Unix()
		}
		m.mu.Unlock()
	}
	_, _ = w.Write([]byte("Ok."))
}

// Stall marks a torrent stalled with an old last_activity for StuckDownloads tests.
func (m *MockServer) Stall(hash string, state string, lastActivity time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.torrents[strings.ToLower(hash)]
	if !ok {
		return false
	}
	if state == "" {
		state = "stalledDL"
	}
	t.State = state
	t.LastActivity = lastActivity.Unix()
	return true
}

func hashFromURL(u string) string {
	if h := ParseMagnetHash(u); h != "" {
		return h
	}
	sum := sha1.Sum([]byte(u))
	return hex.EncodeToString(sum[:])
}

func nameFromURL(u string) string {
	if i := strings.Index(u, "dn="); i >= 0 {
		rest := u[i+3:]
		if j := strings.IndexAny(rest, "&"); j >= 0 {
			rest = rest[:j]
		}
		if rest != "" {
			return rest
		}
	}
	return "fixture-torrent"
}
