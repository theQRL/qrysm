package rpc

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api"
	"github.com/theQRL/qrysm/io/file"
)

func (s *Server) initializeAuthToken() error {
	if s.walletDir == "" {
		return errors.New("wallet directory is required to initialize validator auth token")
	}
	if err := file.MkdirAll(s.walletDir); err != nil {
		return errors.Wrap(err, "could not prepare wallet directory for validator auth token")
	}

	s.authTokenPath = filepath.Join(s.walletDir, api.AuthTokenFileName)
	if file.FileExists(s.authTokenPath) {
		token, err := loadAuthToken(s.authTokenPath)
		if err != nil {
			return err
		}
		s.authToken = token
		return nil
	}

	token, err := api.GenerateRandomHexString()
	if err != nil {
		return errors.Wrap(err, "could not generate validator auth token")
	}
	if err := file.WriteFile(s.authTokenPath, []byte(token+"\n")); err != nil {
		return errors.Wrap(err, "could not persist validator auth token")
	}
	s.authToken = token
	log.WithField("path", s.authTokenPath).Info("Wrote validator auth token")
	return nil
}

func loadAuthToken(path string) (string, error) {
	contents, err := file.ReadFileAsBytes(path)
	if err != nil {
		return "", errors.Wrap(err, "could not read validator auth token")
	}
	token := parseAuthToken(contents)
	if token == "" {
		return "", errors.New("validator auth token file is empty")
	}
	if err := api.ValidateAuthToken(token); err != nil {
		return "", err
	}
	return token, nil
}

func parseAuthToken(contents []byte) string {
	lines := strings.Split(string(contents), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		token := strings.TrimSpace(lines[i])
		if token != "" {
			return token
		}
	}
	return ""
}
