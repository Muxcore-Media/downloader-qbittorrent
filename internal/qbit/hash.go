package qbit

import (
	"strings"
)

// ParseMagnetHash extracts the info-hash from a magnet URI (case-insensitive xt=urn:btih).
func ParseMagnetHash(magnet string) string {
	lower := strings.ToLower(magnet)
	const needle = "xt=urn:btih:"
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return ""
	}
	rest := magnet[idx+len(needle):]
	end := strings.IndexAny(rest, "&")
	if end >= 0 {
		rest = rest[:end]
	}
	return strings.ToLower(strings.TrimSpace(rest))
}
