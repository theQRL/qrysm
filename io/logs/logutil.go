// Package logs creates a Multi writer instance that
// write all logs that are written to stdout.
package logs

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/io/file"
)

func addLogWriter(w io.Writer) {
	mw := io.MultiWriter(logrus.StandardLogger().Out, w)
	logrus.SetOutput(mw)
}

// ConfigurePersistentLogging adds a log-to-file writer. File content is identical to stdout.
func ConfigurePersistentLogging(logFileName string) error {
	logrus.WithField("logFileName", logFileName).Info("Logs will be made persistent")
	if err := file.MkdirAll(filepath.Dir(logFileName)); err != nil {
		return err
	}
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, params.BeaconIoConfig().ReadWritePermissions) // #nosec G304
	if err != nil {
		return err
	}

	addLogWriter(f)

	logrus.Info("File logging initialized")
	return nil
}

// MaskCredentialsLogging masks the url credentials before logging for security purpose
// [scheme:][//[userinfo@]host][/]path[?query][#fragment] -->  [scheme:][//[***]host][/***][#***]
// if the format is not matched nothing is done, string is returned as is.
//
// The masked string is rebuilt from the parsed components rather than by
// substituting text in the original: url.Parse normalises percent-escapes
// (e.g. "p%2fss" becomes "p%2Fss"), so a textual replacement of the parsed
// userinfo could miss the original and leave the password in the log, and a
// fragment following a bare "/" path was left in place.
func MaskCredentialsLogging(currUrl string) string {
	u, err := url.Parse(currUrl)
	if err != nil {
		return currUrl // Not a URL, nothing to do
	}
	// "host:port" parses as scheme "host" with the port as opaque data; there
	// is nothing to mask in it.
	if u.Opaque != "" && u.Host == "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && isAllDigits(u.Opaque) {
		return currUrl
	}
	// Assembled by hand rather than through url.URL.String, which would
	// percent-escape the "***" placeholders in the userinfo.
	var masked strings.Builder
	if u.Scheme != "" {
		masked.WriteString(u.Scheme)
		masked.WriteByte(':')
	}
	if u.Opaque != "" {
		masked.WriteString("***")
	} else {
		if u.Scheme != "" || u.Host != "" || u.User != nil {
			masked.WriteString("//")
		}
		if u.User != nil {
			masked.WriteString("***@")
		}
		masked.WriteString(u.Host)
		switch {
		case (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery:
			// Mask the path and query (they may carry API keys), keep a lone '/'.
			masked.WriteString("/***")
		case u.Path == "/":
			masked.WriteByte('/')
		}
	}
	if u.Fragment != "" || u.RawFragment != "" {
		masked.WriteString("#***")
	}
	return masked.String()
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
