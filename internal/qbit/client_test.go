package qbit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Muxcore-Media/downloader-qbittorrent/internal/qbit"
)

func TestClientAddPauseResumeDeleteOffline(t *testing.T) {
	mock := qbit.NewMockServer("admin", "secret")
	defer mock.Close()

	c := mock.Client()
	magnet := "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&dn=Fixture.Movie"
	if err := c.AddTorrent(context.Background(), magnet, "/downloads", "movies", false); err != nil {
		t.Fatal(err)
	}
	hash := qbit.ParseMagnetHash(magnet)
	got, err := c.GetTorrent(context.Background(), hash)
	if err != nil || got == nil {
		t.Fatalf("get torrent: %+v err=%v", got, err)
	}
	if err := c.Pause(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetTorrent(context.Background(), hash)
	if err != nil || got == nil || qbit.MapState(got.State) != "paused" {
		t.Fatalf("after pause: %+v err=%v", got, err)
	}
	if err := c.Resume(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
	if !mock.Complete(hash) {
		t.Fatal("complete failed")
	}
	got, _ = c.GetTorrent(context.Background(), hash)
	if qbit.MapState(got.State) != "completed" {
		t.Fatalf("want completed, got %s", got.State)
	}
	if len(got.Files) == 0 {
		t.Fatal("expected files populated")
	}
	if err := c.Delete(context.Background(), hash, true); err != nil {
		t.Fatal(err)
	}
	items, _ := c.ListTorrents(context.Background(), "", "")
	if len(items) != 0 {
		t.Fatalf("after delete: %+v", items)
	}
}

func TestClientRejectsBadPassword(t *testing.T) {
	mock := qbit.NewMockServer("admin", "secret")
	defer mock.Close()
	c := &qbit.Client{
		BaseURL: mock.URL(), Username: "admin", Password: "wrong",
		HTTPClient: mock.Server.Client(),
	}
	_, err := c.Version(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestClientRetriesExpiredSID(t *testing.T) {
	var mu sync.Mutex
	authed := map[string]bool{}
	expired := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_ = r.ParseForm()
			if r.Form.Get("password") != "secret" {
				http.Error(w, "Fails.", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid-1", Path: "/"})
			mu.Lock()
			authed["sid-1"] = true
			mu.Unlock()
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			cookie, err := r.Cookie("SID")
			mu.Lock()
			ok := err == nil && authed[cookie.Value]
			if ok && !expired {
				expired = true
				delete(authed, cookie.Value)
				ok = false
			}
			mu.Unlock()
			if !ok {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("v4.6.1"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &qbit.Client{
		BaseURL: srv.URL, Username: "admin", Password: "secret",
		HTTPClient: srv.Client(),
	}
	ver, err := c.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ver, "4.6.1") {
		t.Fatalf("version=%q", ver)
	}
}

func TestClient5xStopStart(t *testing.T) {
	mock := qbit.NewMockServer("admin", "secret")
	defer mock.Close()
	c := mock.Client()
	magnet := "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=FiveX"
	if err := c.AddTorrent(context.Background(), magnet, "/dl", "", true); err != nil {
		t.Fatal(err)
	}
	hash := qbit.ParseMagnetHash(magnet)
	if err := c.Pause(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
	got, _ := c.GetTorrent(context.Background(), hash)
	if qbit.MapState(got.State) != "paused" {
		t.Fatalf("state=%q", got.State)
	}
	if err := c.Resume(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureClientCompletes(t *testing.T) {
	f := qbit.NewFixtureClient()
	hash, tor, err := f.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc&dn=X", "/dl", "tv", false)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" || tor == nil || qbit.MapState(tor.State) != "completed" {
		t.Fatalf("fixture should complete: hash=%q tor=%+v", hash, tor)
	}
}
