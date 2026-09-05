package qbit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureMaterializesMediaFile(t *testing.T) {
	dir := t.TempDir()
	f := NewFixtureClient()
	_, tor, err := f.AddTorrent(context.Background(),
		"magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&dn=Fixture.Movie",
		dir, "movies", false)
	if err != nil {
		t.Fatal(err)
	}
	if tor.ContentPath == "" {
		t.Fatal("expected content path")
	}
	if _, err := os.Stat(tor.ContentPath); err != nil {
		t.Fatalf("content file missing: %v", err)
	}
	if filepath.Ext(tor.ContentPath) != ".mkv" {
		t.Fatalf("unexpected ext: %s", tor.ContentPath)
	}
}

func TestFixtureCopiesMediaSeed(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.mkv")
	payload := []byte("fixture-seed-bytes-for-copy")
	if err := os.WriteFile(seed, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIXTURE_MEDIA_SEED", seed)

	f := NewFixtureClient()
	_, tor, err := f.AddTorrent(context.Background(),
		"magnet:?xt=urn:btih:ABCDEF0123456789ABCDEF0123456789ABCDEF01&dn=Seeded.Movie",
		dir, "movies", false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(tor.ContentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("seed copy mismatch: %q", got)
	}
	if tor.Size != int64(len(payload)) {
		t.Fatalf("size=%d want %d", tor.Size, len(payload))
	}
}

func TestFixtureDeleteRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	f := NewFixtureClient()
	hash, tor, err := f.AddTorrent(context.Background(),
		"magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=Delete.Me",
		dir, "movies", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tor.ContentPath); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if err := f.Delete(context.Background(), hash, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tor.ContentPath); !os.IsNotExist(err) {
		t.Fatalf("content should be removed: %v", err)
	}
}
