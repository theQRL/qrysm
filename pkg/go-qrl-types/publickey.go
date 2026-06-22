package types

type PublicKey interface {
	Marshal() []byte
	Copy() PublicKey
}
