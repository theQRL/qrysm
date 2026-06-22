package types

type Signature interface {
	Verify(msg []byte, pub PublicKey) bool
	Marshal() []byte
}
