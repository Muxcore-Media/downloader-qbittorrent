package internal_test

import (
	"testing"

	"github.com/Muxcore-Media/downloader-qbittorrent/internal"
)

func TestUpdateSettingRebuildsClient(t *testing.T) {
	m := internal.NewModule(internal.Config{
		BaseURL:  "http://127.0.0.1:8080",
		Username: "admin",
		Password: "old",
		Fixture:  false,
	})
	if err := m.UpdateSetting("base_url", "http://127.0.0.1:9090"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("username", "user2"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("password", "********"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("password", "newsecret"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSettingModeReadOnly(t *testing.T) {
	m := internal.NewModule(internal.Config{Fixture: true})
	if err := m.UpdateSetting("mode", "live"); err == nil {
		t.Fatal("expected mode to be read-only")
	}
}

func TestUpdateSettingUnknownKey(t *testing.T) {
	m := internal.NewModule(internal.Config{Fixture: true})
	if err := m.UpdateSetting("nope", "x"); err == nil {
		t.Fatal("expected unknown key error")
	}
}
