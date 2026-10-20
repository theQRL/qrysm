package slashings

import qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"

// IsSurround checks if an attestation, a, is surrounding
// another one, b, based on the Casper FFG surround-vote slashing condition.
//
//	s: source
//	t: target
//
//	a surrounds b if: s_a < s_b and t_b < t_a
func IsSurround(a, b *qrysmpb.IndexedAttestation) bool {
	return a.Data.Source.Epoch < b.Data.Source.Epoch && b.Data.Target.Epoch < a.Data.Target.Epoch
}
