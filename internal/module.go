package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Muxcore-Media/core/sdk/go/module/meshtls"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	cdlv1 "github.com/Muxcore-Media/contracts-downloader/muxcore/downloader/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/core/sdk/go/client"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	manifest "github.com/Muxcore-Media/downloader-qbittorrent"
	"github.com/Muxcore-Media/downloader-qbittorrent/internal/qbit"
)

// EventPublisher emits download.* domain events (test sink or mesh adapter).
type EventPublisher func(ctx context.Context, eventType string, payload []byte) error

type Module struct {
	id       string
	grpcAddr string
	httpAddr string

	cfgMu    sync.RWMutex
	base     string
	username string
	password string
	fixture  bool

	client  *qbit.Client
	fix     *qbit.FixtureClient
	grpcSrv *grpc.Server
	lis     net.Listener
	httpSrv *http.Server

	pubMu     sync.RWMutex
	publish   EventPublisher
	mc        *client.Client
	watched   sync.Map
	pollEvery time.Duration
}

type Config struct {
	ID         string
	BaseURL    string
	Username   string
	Password   string
	GRPCAddr   string
	HTTPAddr   string
	Fixture    bool
	Publish    EventPublisher
	HTTPClient *http.Client
	PollEvery  time.Duration
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func hasLiveURL(base string) bool {
	if strings.TrimSpace(base) != "" {
		return true
	}
	return envFirst("QBIT_URL", "QBITTORRENT_URL") != ""
}

func fixtureEnabled(explicit bool, baseURL string) bool {
	if hasLiveURL(baseURL) {
		return false
	}
	if explicit {
		return true
	}
	if os.Getenv("QBIT_FIXTURE") == "1" || strings.EqualFold(os.Getenv("QBIT_FIXTURE"), "true") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DOWNLOADER_ENGINE"))) {
	case "fixture", "fake":
		return true
	}
	return false
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "downloader-qbittorrent"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = envFirst("QBIT_GRPC_ADDR", "MUXCORE_GRPC_ADDR_OVERRIDE")
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9462"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = envFirst("QBIT_HTTP_ADDR", "MUXCORE_HTTP_ADDR")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9463"
	}
	if v := envFirst("QBIT_URL", "QBITTORRENT_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := envFirst("QBIT_USERNAME", "QBITTORRENT_USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := envFirst("QBIT_PASSWORD", "QBITTORRENT_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if cfg.Username == "" {
		cfg.Username = "admin"
	}
	poll := cfg.PollEvery
	if poll <= 0 {
		poll = 2 * time.Second
	}
	useFixture := fixtureEnabled(cfg.Fixture, cfg.BaseURL)
	m := &Module{
		id:        cfg.ID,
		grpcAddr:  cfg.GRPCAddr,
		httpAddr:  cfg.HTTPAddr,
		base:      cfg.BaseURL,
		username:  cfg.Username,
		password:  cfg.Password,
		fixture:   useFixture,
		publish:   cfg.Publish,
		pollEvery: poll,
	}
	if useFixture {
		m.fix = qbit.NewFixtureClient()
		slog.Info("using fixture qBittorrent backend (no live WebUI)")
	} else {
		m.rebuildClient(cfg.HTTPClient)
	}
	return m
}

func (m *Module) rebuildClient(httpClient *http.Client) {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	m.rebuildClientLocked(httpClient)
}

func (m *Module) rebuildClientLocked(httpClient *http.Client) {
	if m.fixture {
		return
	}
	c := &qbit.Client{BaseURL: m.base, Username: m.username, Password: m.password}
	if httpClient != nil {
		c.HTTPClient = httpClient
	} else if m.client != nil && m.client.HTTPClient != nil {
		c.HTTPClient = m.client.HTTPClient
	}
	m.client = c
}

func (m *Module) SetPublisher(p EventPublisher) {
	m.pubMu.Lock()
	m.publish = p
	m.pubMu.Unlock()
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "qBittorrent Downloader",
		Version:      modulesdk.ManifestVersion(manifest.ManifestJSON),
		Roles:        []string{"downloader"},
		Description:  "qBittorrent WebUI API bridge for torrent downloads",
		Author:       "MuxCore",
		Capabilities: []string{"downloader", "downloader.torrent", "downloader.qbittorrent", "settings"},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/contracts-downloader",
				Interface: "Downloader",
				Version:   "v0.1.0",
			},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error { return nil }

// ListenAddr returns the bound gRPC address after Start (useful for tests with :0).
func (m *Module) ListenAddr() string {
	if m.lis == nil {
		return m.grpcAddr
	}
	return m.lis.Addr().String()
}

// HTTPListenAddr returns the bound health HTTP address after Start.
func (m *Module) HTTPListenAddr() string {
	return m.httpAddr
}

func (m *Module) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	m.grpcAddr = lis.Addr().String()
	srv, err := meshtls.NewServer()
	if err != nil {
		_ = m.lis.Close()
		return fmt.Errorf("gRPC mesh TLS: %w", err)
	}
	m.grpcSrv = srv
	cdlv1.RegisterDownloaderServiceServer(m.grpcSrv, &contractsServer{m: m})
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)

	go func() {
		slog.Info("qbittorrent gRPC listening", "addr", m.grpcAddr, "fixture", m.fixture)
		if err := m.grpcSrv.Serve(lis); err != nil {
			slog.Error("gRPC serve", "error", err)
		}
	}()

	go m.dialCore(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := m.Health(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	httpLis, err := net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen health %s: %w", m.httpAddr, err)
	}
	m.httpAddr = httpLis.Addr().String()
	m.httpSrv = &http.Server{Handler: mux}
	go func() {
		slog.Info("health listening", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("health serve", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	if m.mc != nil {
		if err := m.mc.Close(); err != nil {
			slog.Warn("close mesh client", "error", err)
		}
	}
	return nil
}

func (m *Module) dialCore(ctx context.Context) {
	meshAddr := os.Getenv("MUXCORE_GRPC_ADDR")
	if meshAddr == "" {
		return
	}
	insecureMode := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	var opts []client.Option
	if insecureMode {
		opts = append(opts, client.WithInsecure())
	}
	c, err := client.Dial(meshAddr, opts...)
	if err != nil {
		slog.Warn("qbittorrent: dial core failed", "error", err)
		return
	}
	m.pubMu.Lock()
	m.mc = c
	m.pubMu.Unlock()
	slog.Info("qbittorrent: connected to core mesh", "addr", meshAddr)
}

func (m *Module) Health(ctx context.Context) error {
	if m.fixture {
		return nil
	}
	m.cfgMu.RLock()
	base := m.base
	m.cfgMu.RUnlock()
	if base == "" {
		return nil // unconfigured optional peer
	}
	_, err := m.client.Version(ctx)
	return err
}

func (m *Module) configured() error {
	if m.fixture {
		return nil
	}
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	if m.base == "" {
		return fmt.Errorf("qbittorrent unconfigured: set QBIT_URL / QBITTORRENT_URL (or QBIT_FIXTURE=1 / DOWNLOADER_ENGINE=fixture)")
	}
	return nil
}

func (m *Module) publishDownload(eventType, id, name, savePath, errStr string, files []contracts.DownloadEventFile) {
	if len(files) == 0 {
		files = filesFromStorage(savePath)
	}
	payload, err := json.Marshal(contracts.DownloadEventPayload{
		ID: id, Name: name, SavePath: savePath, Label: "qbittorrent", Error: errStr,
		Files: files,
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m.pubMu.RLock()
	pub := m.publish
	mc := m.mc
	m.pubMu.RUnlock()
	if pub != nil {
		if err := pub(ctx, eventType, payload); err != nil {
			slog.Warn("qbittorrent: publish event failed", "type", eventType, "error", err)
		}
		return
	}
	if mc != nil {
		if err := mc.Events.Publish(ctx, eventType, m.id, payload); err != nil {
			slog.Warn("qbittorrent: publish event failed", "type", eventType, "error", err)
		}
	}
}

func filesFromStorage(storage string) []contracts.DownloadEventFile {
	if storage == "" {
		return nil
	}
	return []contracts.DownloadEventFile{{Path: storage}}
}

func filesFromTorrent(t *qbit.Torrent) []contracts.DownloadEventFile {
	if t == nil {
		return nil
	}
	if t.ContentPath != "" {
		return []contracts.DownloadEventFile{{Path: t.ContentPath}}
	}
	if len(t.Files) > 0 {
		base := t.SavePath
		out := make([]contracts.DownloadEventFile, 0, len(t.Files))
		for _, f := range t.Files {
			path := f.Name
			if base != "" && !strings.HasPrefix(f.Name, "/") {
				path = strings.TrimRight(base, "/") + "/" + f.Name
			}
			out = append(out, contracts.DownloadEventFile{Path: path, Size: f.Size})
		}
		return out
	}
	return filesFromStorage(t.SavePath)
}

func (m *Module) watchTorrent(hash string) {
	if hash == "" {
		return
	}
	if _, loaded := m.watched.LoadOrStore(hash, struct{}{}); loaded {
		return
	}
	go func() {
		ticker := time.NewTicker(m.pollEvery)
		defer ticker.Stop()
		defer m.watched.Delete(hash)
		deadline := time.Now().Add(72 * time.Hour)
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			var t *qbit.Torrent
			var err error
			if m.fixture {
				t, err = m.fix.Get(ctx, hash)
			} else {
				t, err = m.client.GetTorrent(ctx, hash)
			}
			cancel()
			if err == nil && t != nil {
				st := qbit.MapState(t.State)
				switch st {
				case "completed":
					m.publishDownload(contracts.EventDownloadCompleted, t.Hash, t.Name, t.SavePath, "", filesFromTorrent(t))
					return
				case "failed":
					m.publishDownload(contracts.EventDownloadFailed, t.Hash, t.Name, t.SavePath, t.State, filesFromTorrent(t))
					return
				}
			}
			<-ticker.C
		}
	}()
}

type contractsServer struct {
	cdlv1.UnimplementedDownloaderServiceServer
	m *Module
}

func (s *contractsServer) AddTorrent(ctx context.Context, req *cdlv1.AddTorrentRequest) (*cdlv1.AddTorrentResponse, error) {
	if err := s.m.configured(); err != nil {
		return nil, err
	}
	url := req.GetTorrentUrl()
	if url == "" {
		return nil, fmt.Errorf("torrent_url required")
	}
	if s.m.fixture {
		hash, t, err := s.m.fix.AddTorrent(ctx, url, req.GetSavePath(), req.GetCategory(), req.GetPaused())
		if err != nil {
			return nil, err
		}
		s.m.publishDownload(contracts.EventDownloadStarted, hash, t.Name, t.SavePath, "", filesFromTorrent(t))
		if !req.GetPaused() {
			s.m.publishDownload(contracts.EventDownloadCompleted, hash, t.Name, t.SavePath, "", filesFromTorrent(t))
		} else {
			s.m.watchTorrent(hash)
		}
		return &cdlv1.AddTorrentResponse{TorrentId: hash, Name: t.Name, InfoHash: hash}, nil
	}
	if err := s.m.client.AddTorrent(ctx, url, req.GetSavePath(), req.GetCategory(), req.GetPaused()); err != nil {
		return nil, err
	}
	hash := qbit.ResolveHash(url)
	if hash == "" {
		return nil, fmt.Errorf("torrent added but magnet info-hash could not be parsed")
	}
	match, err := s.m.client.GetTorrent(ctx, hash)
	if err != nil {
		return nil, err
	}
	if match == nil {
		return nil, fmt.Errorf("torrent added but hash %q not found in qBittorrent", hash)
	}
	s.m.publishDownload(contracts.EventDownloadStarted, match.Hash, match.Name, match.SavePath, "", filesFromTorrent(match))
	s.m.watchTorrent(hash)
	return &cdlv1.AddTorrentResponse{
		TorrentId: match.Hash,
		Name:      match.Name,
		InfoHash:  match.Hash,
	}, nil
}

func (s *contractsServer) RemoveTorrent(ctx context.Context, req *cdlv1.RemoveTorrentRequest) (*cdlv1.RemoveTorrentResponse, error) {
	if err := s.m.configured(); err != nil {
		return nil, err
	}
	id := req.GetTorrentId()
	if id == "" {
		return nil, fmt.Errorf("torrent_id required")
	}
	var err error
	if s.m.fixture {
		err = s.m.fix.Delete(ctx, id, req.GetDeleteFiles())
	} else {
		err = s.m.client.Delete(ctx, id, req.GetDeleteFiles())
	}
	if err != nil {
		return nil, err
	}
	return &cdlv1.RemoveTorrentResponse{Success: true}, nil
}

func (s *contractsServer) GetTorrent(ctx context.Context, req *cdlv1.GetTorrentRequest) (*cdlv1.GetTorrentResponse, error) {
	if err := s.m.configured(); err != nil {
		return nil, err
	}
	id := req.GetTorrentId()
	var t *qbit.Torrent
	var err error
	if s.m.fixture {
		t, err = s.m.fix.Get(ctx, id)
	} else {
		t, err = s.m.client.GetTorrent(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("torrent %q not found", id)
	}
	return &cdlv1.GetTorrentResponse{Torrent: mapTorrent(t)}, nil
}

func (s *contractsServer) ListTorrents(ctx context.Context, req *cdlv1.ListTorrentsRequest) (*cdlv1.ListTorrentsResponse, error) {
	if err := s.m.configured(); err != nil {
		return &cdlv1.ListTorrentsResponse{}, nil
	}
	var list []qbit.Torrent
	var err error
	status := listStatusFilter(req.GetStatus())
	if s.m.fixture {
		list, err = s.m.fix.List(ctx, req.GetCategory(), status)
	} else {
		list, err = s.m.client.ListTorrents(ctx, req.GetCategory(), status)
	}
	if err != nil {
		return nil, err
	}
	out := make([]*cdlv1.TorrentInfo, 0, len(list))
	for i := range list {
		out = append(out, mapTorrent(&list[i]))
	}
	return &cdlv1.ListTorrentsResponse{Torrents: out}, nil
}

func (s *contractsServer) PauseTorrent(ctx context.Context, req *cdlv1.PauseTorrentRequest) (*cdlv1.PauseTorrentResponse, error) {
	if err := s.m.configured(); err != nil {
		return nil, err
	}
	id := req.GetTorrentId()
	var err error
	if s.m.fixture {
		err = s.m.fix.Pause(ctx, id)
	} else {
		err = s.m.client.Pause(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return &cdlv1.PauseTorrentResponse{Success: true}, nil
}

func (s *contractsServer) ResumeTorrent(ctx context.Context, req *cdlv1.ResumeTorrentRequest) (*cdlv1.ResumeTorrentResponse, error) {
	if err := s.m.configured(); err != nil {
		return nil, err
	}
	id := req.GetTorrentId()
	var err error
	if s.m.fixture {
		err = s.m.fix.Resume(ctx, id)
	} else {
		err = s.m.client.Resume(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	s.m.watchTorrent(id)
	return &cdlv1.ResumeTorrentResponse{Success: true}, nil
}

func (s *contractsServer) GetCapabilities(context.Context, *cdlv1.GetCapabilitiesRequest) (*cdlv1.GetCapabilitiesResponse, error) {
	return &cdlv1.GetCapabilitiesResponse{
		SupportsCategories:    true,
		SupportsPausing:       true,
		SupportsFileSelection: false,
		SupportedProtocols:    []string{"torrent", "magnet"},
	}, nil
}

func mapTorrent(t *qbit.Torrent) *cdlv1.TorrentInfo {
	if t == nil {
		return nil
	}
	info := &cdlv1.TorrentInfo{
		Id:            t.Hash,
		Name:          t.Name,
		InfoHash:      t.Hash,
		Size:          t.Size,
		Downloaded:    t.Downloaded,
		Uploaded:      t.Uploaded,
		Progress:      t.Progress,
		DownloadSpeed: t.Dlspeed,
		UploadSpeed:   t.Upspeed,
		Seeders:       int32(t.NumSeeds),
		Leechers:      int32(t.NumLeechs),
		Status:        mapProtoStatus(qbit.MapState(t.State)),
		SavePath:      t.SavePath,
		Category:      t.Category,
	}
	if len(t.Files) > 0 {
		info.Files = make([]*cdlv1.TorrentFile, 0, len(t.Files))
		for _, f := range t.Files {
			info.Files = append(info.Files, &cdlv1.TorrentFile{
				Path:       f.Name,
				Size:       f.Size,
				Downloaded: int64(float64(f.Size) * f.Progress),
				Wanted:     true,
			})
		}
	}
	if t.AddedOn > 0 {
		info.AddedAt = timestamppb.New(time.Unix(t.AddedOn, 0).UTC())
	}
	if t.CompletionOn > 0 {
		info.CompletedAt = timestamppb.New(time.Unix(t.CompletionOn, 0).UTC())
	}
	return info
}

func listStatusFilter(st cdlv1.TorrentStatus) string {
	switch st {
	case cdlv1.TorrentStatus_TORRENT_STATUS_DOWNLOADING:
		return "downloading"
	case cdlv1.TorrentStatus_TORRENT_STATUS_PAUSED:
		return "paused"
	case cdlv1.TorrentStatus_TORRENT_STATUS_COMPLETED:
		return "completed"
	case cdlv1.TorrentStatus_TORRENT_STATUS_FAILED:
		return "failed"
	case cdlv1.TorrentStatus_TORRENT_STATUS_STALLED:
		return "stalled"
	default:
		return ""
	}
}

func mapProtoStatus(label string) cdlv1.TorrentStatus {
	switch label {
	case "completed":
		return cdlv1.TorrentStatus_TORRENT_STATUS_COMPLETED
	case "downloading":
		return cdlv1.TorrentStatus_TORRENT_STATUS_DOWNLOADING
	case "paused":
		return cdlv1.TorrentStatus_TORRENT_STATUS_PAUSED
	case "failed":
		return cdlv1.TorrentStatus_TORRENT_STATUS_FAILED
	case "stalled":
		return cdlv1.TorrentStatus_TORRENT_STATUS_STALLED
	default:
		return cdlv1.TorrentStatus_TORRENT_STATUS_UNKNOWN
	}
}
