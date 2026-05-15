package qbittorrent

import (
	"context"
	"fmt"
	"os"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

type Module struct {
	client *Client
	addr   string
	user   string
	pass   string
}

func NewModule() *Module {
	return &Module{
		addr: envOrDefault("MUXCORE_QBITTORRENT_ADDR", "http://localhost:8080"),
		user: envOrDefault("MUXCORE_QBITTORRENT_USER", "admin"),
		pass: os.Getenv("MUXCORE_QBITTORRENT_PASS"),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           "downloader-qbittorrent",
		Name:         "qBittorrent",
		Version:      "1.0.0",
		Kind:         contracts.ModuleKindDownloader,
		Description:  "qBittorrent download client connector",
		Author:       "MuxCore",
		Capabilities: []string{"downloader.torrent", "downloader.qbittorrent"},
	}
}

func (m *Module) Init(ctx context.Context) error {
	m.client = NewClient(m.addr, m.user, m.pass)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if err := m.client.Login(ctx); err != nil {
		return fmt.Errorf("qbittorrent login: %w", err)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error { return nil }

func (m *Module) Health(ctx context.Context) error {
	_, err := m.client.List(ctx)
	return err
}

func (m *Module) Add(ctx context.Context, task contracts.DownloadTask) (string, error) {
	return m.client.Add(ctx, task)
}

func (m *Module) Remove(ctx context.Context, id string, deleteData bool) error {
	return m.client.Remove(ctx, id, deleteData)
}

func (m *Module) Pause(ctx context.Context, id string) error {
	return m.client.Pause(ctx, id)
}

func (m *Module) Resume(ctx context.Context, id string) error {
	return m.client.Resume(ctx, id)
}

func (m *Module) Status(ctx context.Context, id string) (contracts.DownloadInfo, error) {
	return m.client.Status(ctx, id)
}

func (m *Module) List(ctx context.Context) ([]contracts.DownloadInfo, error) {
	return m.client.List(ctx)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
