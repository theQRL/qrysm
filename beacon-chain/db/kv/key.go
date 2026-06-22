package kv

import "bytes"

func hasZondKey(enc []byte) bool {
	if len(zondKey) >= len(enc) {
		return false
	}
	return bytes.Equal(enc[:len(zondKey)], zondKey)
}

func hasZondBlindKey(enc []byte) bool {
	if len(zondBlindKey) >= len(enc) {
		return false
	}
	return bytes.Equal(enc[:len(zondBlindKey)], zondBlindKey)
}
