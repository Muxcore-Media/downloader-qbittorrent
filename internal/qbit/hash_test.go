package qbit

import "testing"

func TestParseMagnetHash(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:AbCdEf0123456789AbCdEf0123456789AbCdEf01&dn=Movie"
	got := ParseMagnetHash(magnet)
	want := "abcdef0123456789abcdef0123456789abcdef01"
	if got != want {
		t.Fatalf("hash=%q want %q", got, want)
	}
	if ParseMagnetHash("http://example.com/a.torrent") != "" {
		t.Fatal("expected empty for http URL")
	}
}
