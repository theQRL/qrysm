package logs

import (
	"strings"
	"testing"

	"github.com/theQRL/qrysm/testing/require"
)

var urltests = []struct {
	url       string
	maskedUrl string
}{
	{"https://a:b@xyz.net", "https://***@xyz.net"},
	{"https://eth-goerli.alchemyapi.io/v2/tOZG5mjl3.zl_nZdZTNIBUzsDq62R_dkOtY",
		"https://eth-goerli.alchemyapi.io/***"},
	{"https://google.com/search?q=golang", "https://google.com/***"},
	{"https://user@example.com/foo%2fbar", "https://***@example.com/***"},
	{"http://john@example.com/#x/y%2Fz", "http://***@example.com/#***"},
	{"https://me:pass@example.com/foo/bar?x=1&y=2", "https://***@example.com/***"},
	// Non-canonical percent-escapes in the userinfo: url.Parse re-encodes
	// "%2f" as "%2F", so a textual replacement of the parsed userinfo used to
	// miss and leak the password.
	{"https://user:p%2fss@example.com/rpc", "https://***@example.com/***"},
	{"https://user:pa%3ass%40word@example.com", "https://***@example.com"},
	// Fragments after a bare "/" or no path used to be left in place.
	{"https://example.com/#secret", "https://example.com/#***"},
	{"https://example.com#secret", "https://example.com#***"},
	{"https://user:pass@example.com/?key=abc#token", "https://***@example.com/***#***"},
	// Opaque URLs and plain hosts.
	{"mailto:someone@example.com", "mailto:***"},
	{"http://localhost:8545", "http://localhost:8545"},
	{"http://localhost:8545/", "http://localhost:8545/"},
	// Not a URL: returned unchanged.
	{"localhost:8545", "localhost:8545"},
}

func TestMaskCredentialsLogging(t *testing.T) {
	for _, test := range urltests {
		require.Equal(t, test.maskedUrl, MaskCredentialsLogging(test.url), "input %q", test.url)
	}
}

// TestMaskCredentialsLogging_NeverContainsSecret checks, independently of the
// exact masked form, that no part of the userinfo, path, query or fragment
// survives masking.
func TestMaskCredentialsLogging_NeverContainsSecret(t *testing.T) {
	for _, secret := range []string{"p%2fss", "pa%3ass%40word", "tOZG5mjl3", "q=golang", "secret", "token", "someone"} {
		for _, test := range urltests {
			if !strings.Contains(test.url, secret) {
				continue
			}
			masked := MaskCredentialsLogging(test.url)
			require.Equal(t, false, strings.Contains(masked, secret), "secret %q leaked into %q", secret, masked)
		}
	}
}
