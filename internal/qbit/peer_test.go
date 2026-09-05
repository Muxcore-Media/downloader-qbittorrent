package qbit_test

import (
	"context"
	"testing"
	"time"

	"github.com/Muxcore-Media/downloader-qbittorrent/internal/qbit"
)

func TestClientCategoriesAndLimits(t *testing.T) {
	mock := qbit.NewMockServer("admin", "secret")
	defer mock.Close()
	c := mock.Client()

	if err := c.CreateCategory(context.Background(), "movies", "/downloads/movies"); err != nil {
		t.Fatal(err)
	}
	cats, err := c.GetCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cats["movies"].SavePath != "/downloads/movies" {
		t.Fatalf("categories: %+v", cats)
	}
	if err := c.RenameCategory(context.Background(), "movies", "films"); err != nil {
		t.Fatal(err)
	}
	cats, _ = c.GetCategories(context.Background())
	if _, ok := cats["films"]; !ok {
		t.Fatalf("rename failed: %+v", cats)
	}
	if err := c.DeleteCategory(context.Background(), "films"); err != nil {
		t.Fatal(err)
	}
	cats, _ = c.GetCategories(context.Background())
	if len(cats) != 0 {
		t.Fatalf("expected empty categories: %+v", cats)
	}

	if err := c.SetDownloadLimit(context.Background(), 1024*1024); err != nil {
		t.Fatal(err)
	}
	if err := c.SetUploadLimit(context.Background(), 512*1024); err != nil {
		t.Fatal(err)
	}
	info, err := c.GetTransferInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.DownloadLimit != 1024*1024 || info.UploadLimit != 512*1024 {
		t.Fatalf("limits: %+v", info)
	}
}

func TestClientStuckDownloadsAndReAdd(t *testing.T) {
	mock := qbit.NewMockServer("admin", "secret")
	defer mock.Close()
	c := mock.Client()
	magnet := "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=Stuck.Movie"
	if err := c.AddTorrent(context.Background(), magnet, "/dl", "tv", false); err != nil {
		t.Fatal(err)
	}
	hash := qbit.ParseMagnetHash(magnet)
	mock.Stall(hash, "stalledDL", time.Now().Add(-20*time.Minute))

	stuck, err := c.StuckDownloads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stuck) != 1 || stuck[0].Hash != hash {
		t.Fatalf("stuck=%+v", stuck)
	}

	if err := c.ReAddTorrent(context.Background(), magnet, "/dl", "tv"); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetTorrent(context.Background(), hash)
	if err != nil || got == nil {
		t.Fatalf("re-added torrent missing: %+v err=%v", got, err)
	}
	if got.State != "checkingDL" {
		t.Fatalf("expected recheck state, got %q", got.State)
	}
}

func TestFixturePeerHelpers(t *testing.T) {
	f := qbit.NewFixtureClient()
	if err := f.CreateCategory(context.Background(), "anime", "/anime"); err != nil {
		t.Fatal(err)
	}
	cats, err := f.GetCategories(context.Background())
	if err != nil || cats["anime"].SavePath != "/anime" {
		t.Fatalf("categories: %+v err=%v", cats, err)
	}
	hash, _, err := f.AddTorrent(context.Background(),
		"magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Retry", "/dl", "anime", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.MarkStuck(hash, "error", time.Now().Add(-16*time.Minute)); err != nil {
		t.Fatal(err)
	}
	stuck, err := f.StuckDownloads(context.Background())
	if err != nil || len(stuck) != 1 {
		t.Fatalf("stuck=%+v err=%v", stuck, err)
	}
	magnet := "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Retry"
	if err := f.ReAddTorrent(context.Background(), magnet, "/dl", "anime"); err != nil {
		t.Fatal(err)
	}
}
