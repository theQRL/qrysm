package rpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theQRL/qrysm/api"
	"github.com/theQRL/qrysm/io/file"
	"github.com/theQRL/qrysm/testing/require"
)

func TestServer_initializeAuthToken_GeneratesAndPersists(t *testing.T) {
	walletDir := filepath.Join(t.TempDir(), "wallet")
	require.NoError(t, file.MkdirAll(walletDir))
	s := &Server{walletDir: walletDir}

	require.NoError(t, s.initializeAuthToken())
	require.Equal(t, filepath.Join(walletDir, api.AuthTokenFileName), s.authTokenPath)
	require.NotEqual(t, "", s.authToken)
	require.NoError(t, api.ValidateAuthToken(s.authToken))

	contents, err := os.ReadFile(s.authTokenPath)
	require.NoError(t, err)
	require.Equal(t, s.authToken, strings.TrimSpace(string(contents)))
}

func TestServer_initializeAuthToken_LoadsExistingToken(t *testing.T) {
	walletDir := filepath.Join(t.TempDir(), "wallet")
	require.NoError(t, file.MkdirAll(walletDir))
	token := "0x" + strings.Repeat("22", 32)
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)
	require.NoError(t, os.WriteFile(authTokenPath, []byte("legacy-secret\n"+token+"\n"), 0o600))

	s := &Server{walletDir: walletDir}
	require.NoError(t, s.initializeAuthToken())
	require.Equal(t, token, s.authToken)
	require.Equal(t, authTokenPath, s.authTokenPath)
}

func TestServer_initializeAuthToken_RejectsInvalidExistingToken(t *testing.T) {
	walletDir := filepath.Join(t.TempDir(), "wallet")
	require.NoError(t, file.MkdirAll(walletDir))
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)
	require.NoError(t, os.WriteFile(authTokenPath, []byte("not-a-token\n"), 0o600))

	s := &Server{walletDir: walletDir}
	err := s.initializeAuthToken()
	require.ErrorContains(t, "invalid auth token", err)
}
