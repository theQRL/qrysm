package dilithium

import "github.com/prysmaticlabs/prysm/v4/crypto/bls/common"

// PublicKey represents a BLS public key.
type PublicKey = common.PublicKey

// DilithiumKey represents a BLS secret or private key.
type DilithiumKey = common.SecretKey

// Signature represents a BLS signature.
type Signature = common.Signature
