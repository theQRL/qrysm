package common

import (
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

// ZeroSecretKey represents a zero secret key.
var ZeroSecretKey = [32]byte{}

// InfinitePublicKey represents an infinite public key (G1 Point at Infinity).
var InfinitePublicKey = [dilithium2.CryptoPublicKeyBytes]byte{0xC0}

// InfiniteSignature represents an infinite signature (G2 Point at Infinity).
var InfiniteSignature = [dilithium2.CryptoBytes]byte{0xC0}
