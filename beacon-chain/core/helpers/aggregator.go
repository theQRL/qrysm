package helpers

import (
	"encoding/binary"
	"fmt"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/hash"
)

// AggregatorSelectionSeed binds the epoch's lookahead RANDAO seed to this chain.
// It must be computed from the same state view as the associated duties, and
// refreshed with those duties after a reorganization.
func AggregatorSelectionSeed(st state.ReadOnlyBeaconState, epoch primitives.Epoch) ([32]byte, error) {
	seed, err := Seed(st, epoch, params.BeaconConfig().DomainSelectionProof)
	if err != nil {
		return [32]byte{}, err
	}
	genesisRoot := st.GenesisValidatorsRoot()
	if len(genesisRoot) != 32 {
		return [32]byte{}, fmt.Errorf("genesis validators root must be 32 bytes")
	}
	input := append([]byte("qrysm/aggregator-selection/v1"), genesisRoot...)
	input = append(input, seed[:]...)
	return hash.Hash(input), nil
}

// IsAggregatorSelected samples a registered validator for a specific duty. All
// inputs have fixed lengths and come from the chain state or assigned duty;
// signature bytes never influence eligibility. ML-DSA permits multiple valid
// signatures for the same message, so hashing a signature allows offline retries.
// Callers must also enforce the validator's membership in the assigned committee.
//
// TODO: Restore private aggregator eligibility with a reviewed post-quantum
// scheme that has a verifiably unique output per registered key and duty. Use
// that verified output for selection. Deterministic ML-DSA signing alone cannot
// enforce uniqueness against a modified signer. This public lottery is the
// interim protocol and makes aggregator identities predictable in advance.
func IsAggregatorSelected(seed []byte, role [4]byte, slot primitives.Slot, committeeIndex uint64, validatorIndex primitives.ValidatorIndex, committeeSize, targetAggregators uint64) (bool, error) {
	if len(seed) != 32 {
		return false, fmt.Errorf("aggregator selection seed must be 32 bytes")
	}
	if committeeSize == 0 || targetAggregators == 0 {
		return false, fmt.Errorf("committee size and target aggregators must be nonzero")
	}
	var input [60]byte
	copy(input[:32], seed)
	copy(input[32:36], role[:])
	binary.LittleEndian.PutUint64(input[36:44], uint64(slot))
	binary.LittleEndian.PutUint64(input[44:52], committeeIndex)
	binary.LittleEndian.PutUint64(input[52:60], uint64(validatorIndex))
	draw := hash.Hash(input[:])
	modulo := max(1, committeeSize/targetAggregators)
	return binary.LittleEndian.Uint64(draw[:8])%modulo == 0, nil
}
