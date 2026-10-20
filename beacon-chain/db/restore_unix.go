//go:build unix

package db

import (
	"os"
	"syscall"
)

// linkCount returns the number of directory entries referring to the file.
func linkCount(info os.FileInfo) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Nlink), true
}
