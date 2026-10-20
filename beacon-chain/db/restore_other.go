//go:build !unix

package db

import "os"

// linkCount is not available on this platform.
func linkCount(_ os.FileInfo) (uint64, bool) {
	return 0, false
}
