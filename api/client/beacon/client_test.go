package beacon

import (
	"net/url"
	"testing"

	"github.com/theQRL/qrysm/api/client"
	"github.com/theQRL/qrysm/testing/require"
)

func TestParseNodeVersion(t *testing.T) {
	cases := []struct {
		name string
		v    string
		err  error
		nv   *NodeVersion
	}{
		{
			name: "empty string",
			v:    "",
			err:  client.ErrInvalidNodeVersion,
		},
		{
			name: "Qrysm as the version string",
			v:    "Qrysm",
			err:  client.ErrInvalidNodeVersion,
		},
		{
			name: "semver only",
			v:    "v2.0.6",
			err:  client.ErrInvalidNodeVersion,
		},
		{
			name: "complete version",
			v:    "Qrysm/v2.0.6 (linux amd64)",
			nv: &NodeVersion{
				implementation: "Qrysm",
				semver:         "v2.0.6",
				systemInfo:     "(linux amd64)",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nv, err := parseNodeVersion(c.v)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.DeepEqual(t, c.nv, nv)
			}
		})
	}
}

func TestValidHostname(t *testing.T) {
	cases := []struct {
		name    string
		hostArg string
		path    string
		joined  string
		err     error
	}{
		{
			name:    "hostname without port",
			hostArg: "mydomain.org",
			err:     client.ErrMalformedHostname,
		},
		{
			name:    "hostname with port",
			hostArg: "mydomain.org:3500",
			path:    getNodeVersionPath,
			joined:  "http://mydomain.org:3500/zond/v1/node/version",
		},
		{
			name:    "https scheme, hostname with port",
			hostArg: "https://mydomain.org:3500",
			path:    getNodeVersionPath,
			joined:  "https://mydomain.org:3500/zond/v1/node/version",
		},
		{
			name:    "http scheme, hostname without port",
			hostArg: "http://mydomain.org",
			path:    getNodeVersionPath,
			joined:  "http://mydomain.org/zond/v1/node/version",
		},
		{
			name:    "http scheme, trailing slash, hostname without port",
			hostArg: "http://mydomain.org/",
			path:    getNodeVersionPath,
			joined:  "http://mydomain.org/zond/v1/node/version",
		},
		{
			name:    "http scheme, hostname with basic auth creds and no port",
			hostArg: "http://username:pass@mydomain.org/",
			path:    getNodeVersionPath,
			joined:  "http://username:pass@mydomain.org/zond/v1/node/version",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl, err := NewClient(c.hostArg)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.joined, cl.BaseURL().ResolveReference(&url.URL{Path: c.path}).String())
		})
	}
}
