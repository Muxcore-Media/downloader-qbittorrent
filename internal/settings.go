package internal

import (
	"fmt"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/downloader-qbittorrent/internal/qbit"
)

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	pass := m.password
	if pass != "" {
		pass = "********"
	}
	mode := "live"
	if m.fixture {
		mode = "fixture"
	}
	return []contracts.SettingDef{
		{
			Key: "base_url", Label: "qBittorrent URL", Type: contracts.SettingTypeString,
			Value: m.base, Description: "e.g. http://127.0.0.1:8080 (operator opt-in; leave empty for soft-empty). Ignored when fixture.", Group: "Connection",
		},
		{
			Key: "username", Label: "Username", Type: contracts.SettingTypeString,
			Value: m.username, Description: "WebUI username (default admin)", Group: "Connection",
		},
		{
			Key: "password", Label: "Password", Type: contracts.SettingTypeSecret,
			Value: pass, Description: "WebUI password (operator opt-in; never required for CI)", Group: "Connection",
		},
		{
			Key: "mode", Label: "Mode", Type: contracts.SettingTypeString,
			Value: mode, Description: "fixture when DOWNLOADER_ENGINE=fixture or QBIT_FIXTURE=1", Group: "Connection",
		},
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	switch key {
	case "base_url":
		if value != "" {
			if err := qbit.ValidateBaseURL(value); err != nil {
				return fmt.Errorf("base_url rejected: %w", err)
			}
		}
		m.base = value
	case "username":
		if value != "" {
			m.username = value
		}
	case "password":
		if value != "" && value != "********" {
			m.password = value
		}
	case "mode":
		// read-only via env
		return fmt.Errorf("mode is controlled by DOWNLOADER_ENGINE / QBIT_FIXTURE")
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	if !m.fixture {
		m.rebuildClientLocked(nil)
	}
	return nil
}
