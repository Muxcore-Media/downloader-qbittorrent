package qbit

import (
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/Muxcore-Media/core/sdk/go/module/netguard"
)

// BaseURLOptions are the netguard options for the operator-configured
// qBittorrent WebUI endpoint (NFR-SEC-009 / RULE-VAL-2): a LAN or loopback
// service is legitimate, but cloud-metadata and link-local targets are not.
func BaseURLOptions() netguard.Options {
	return netguard.Options{AllowPrivate: true, AllowLoopback: true, Timeout: 30 * time.Second}
}

// ValidateBaseURL rejects a WebUI base URL that is not http(s) or that points
// at a metadata/link-local/unspecified address.
func ValidateBaseURL(raw string) error {
	return netguard.ValidateURL(raw, netguard.Integration, BaseURLOptions())
}

// newGuardedHTTPClient is the default WebUI client: dial-time and redirect
// checks under the Integration profile, with a cookie jar for the session.
func newGuardedHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	c := netguard.NewClient(netguard.Integration, BaseURLOptions())
	c.Jar = jar
	return c
}
