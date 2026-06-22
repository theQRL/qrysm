package util

import (
	"context"
	"errors"
	"fmt"
	"math"

	log "github.com/sirupsen/logrus"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/crypto/rand"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/time/slots"
)

// NewAttestation creates an attestation block with minimum marshalable fields.
func NewAttestation() *qrysmpb.Attestation {
	return &qrysmpb.Attestation{
		AggregationBits: bitfield.Bitlist{0b1101},
		Data: &qrysmpb.AttestationData{
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Source: &qrysmpb.Checkpoint{
				Root: make([]byte, fieldparams.RootLength),
			},
			Target: &qrysmpb.Checkpoint{
				Root: make([]byte, fieldparams.RootLength),
			},
		},
		Signatures: [][]byte{make([]byte, field_params.MLDSA87SignatureLength), make([]byte, field_params.MLDSA87SignatureLength)},
	}
}

// GenerateAttestations creates attestations that are entirely valid, for all
// the committees of the current state slot. This function expects attestations
// requested to be cleanly divisible by committees per slot. If there is 1 committee
// in the slot, and numToGen is set to 4, then it will return 4 attestations
// for the same data with their aggregation bits split uniformly.
//
// If you request 4 attestations, but there are 8 committees, you will get 4 fully aggregated attestations.
func GenerateAttestations(
	bState state.BeaconState, privs []ml_dsa_87.MLDSA87Key, numToGen uint64, slot primitives.Slot, randomRoot bool,
) ([]*qrysmpb.Attestation, error) {
	var attestations []*qrysmpb.Attestation
	generateHeadState := false
	bState = bState.Copy()
	if slot > bState.Slot() {
		// Going back a slot here so there's no inclusion delay issues.
		slot--
		generateHeadState = true
	}
	currentEpoch := slots.ToEpoch(slot)

	targetRoot := make([]byte, fieldparams.RootLength)
	var headRoot []byte
	var err error
	// Only calculate head state if its an attestation for the current slot or future slot.
	if generateHeadState || slot == bState.Slot() {
		var headState state.BeaconState
		switch bState.Version() {
		case version.Zond:
			pbState, err := state_native.ProtobufBeaconStateZond(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeZond(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		default:
			return nil, errors.New("state type isn't supported")
		}

		headState, err = transition.ProcessSlots(context.Background(), headState, slot+1)
		if err != nil {
			return nil, err
		}
		headRoot, err = helpers.BlockRootAtSlot(headState, slot)
		if err != nil {
			return nil, err
		}
		targetRoot, err = helpers.BlockRoot(headState, currentEpoch)
		if err != nil {
			return nil, err
		}
	} else {
		headRoot, err = helpers.BlockRootAtSlot(bState, slot)
		if err != nil {
			return nil, err
		}
	}
	if randomRoot {
		randGen := rand.NewDeterministicGenerator()
		b := make([]byte, fieldparams.RootLength)
		_, err := randGen.Read(b)
		if err != nil {
			return nil, err
		}
		headRoot = b
	}

	activeValidatorCount, err := helpers.ActiveValidatorCount(context.Background(), bState, currentEpoch)
	if err != nil {
		return nil, err
	}
	committeesPerSlot := helpers.SlotCommitteeCount(activeValidatorCount)

	if numToGen < committeesPerSlot {
		log.Printf(
			"Warning: %d attestations requested is less than %d committees in current slot, not all validators will be attesting.",
			numToGen,
			committeesPerSlot,
		)
	} else if numToGen > committeesPerSlot {
		log.Printf(
			"Warning: %d attestations requested are more than %d committees in current slot, attestations will not be perfectly efficient.",
			numToGen,
			committeesPerSlot,
		)
	}

	attsPerCommittee := math.Max(float64(numToGen/committeesPerSlot), 1)
	if math.Trunc(attsPerCommittee) != attsPerCommittee {
		return nil, fmt.Errorf(
			"requested attestations %d must be easily divisible by committees in slot %d, calculated %f",
			numToGen,
			committeesPerSlot,
			attsPerCommittee,
		)
	}

	domain, err := signing.Domain(bState.Fork(), currentEpoch, params.BeaconConfig().DomainBeaconAttester, bState.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	for c := primitives.CommitteeIndex(0); uint64(c) < committeesPerSlot && uint64(c) < numToGen; c++ {
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), bState, slot, c)
		if err != nil {
			return nil, err
		}

		attData := &qrysmpb.AttestationData{
			Slot:            slot,
			CommitteeIndex:  c,
			BeaconBlockRoot: headRoot,
			Source:          bState.CurrentJustifiedCheckpoint(),
			Target: &qrysmpb.Checkpoint{
				Epoch: currentEpoch,
				Root:  targetRoot,
			},
		}

		dataRoot, err := signing.ComputeSigningRoot(attData, domain)
		if err != nil {
			return nil, err
		}

		committeeSize := uint64(len(committee))
		bitsPerAtt := committeeSize / uint64(attsPerCommittee)
		for i := uint64(0); i < committeeSize; i += bitsPerAtt {
			aggregationBits := bitfield.NewBitlist(committeeSize)
			sigs := make([][]byte, 0)
			for b := i; b < i+bitsPerAtt; b++ {
				aggregationBits.SetBitAt(b, true)
				sigs = append(sigs, privs[committee[b]].Sign(dataRoot[:]).Marshal())
			}

			att := &qrysmpb.Attestation{
				Data:            attData,
				AggregationBits: aggregationBits,
				Signatures:      sigs,
			}
			attestations = append(attestations, att)
		}
	}
	return attestations, nil
}

// HydrateAttestation hydrates an attestation object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateAttestation(a *qrysmpb.Attestation) *qrysmpb.Attestation {
	if a.Signatures == nil {
		sig := make([]byte, 4627)
		a.Signatures = [][]byte{sig}
	}
	if a.AggregationBits == nil {
		a.AggregationBits = make([]byte, 1)
	}
	if a.Data == nil {
		a.Data = &qrysmpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}

// HydrateV1Attestation hydrates a v1 attestation object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1Attestation(a *qrlpb.Attestation) *qrlpb.Attestation {
	if a.Signatures == nil {
		sig := make([]byte, 4627)
		a.Signatures = [][]byte{sig}
	}
	if a.AggregationBits == nil {
		a.AggregationBits = make([]byte, 1)
	}
	if a.Data == nil {
		a.Data = &qrlpb.AttestationData{}
	}
	a.Data = HydrateV1AttestationData(a.Data)
	return a
}

// HydrateAttestationData hydrates an attestation data object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateAttestationData(d *qrysmpb.AttestationData) *qrysmpb.AttestationData {
	if d.BeaconBlockRoot == nil {
		d.BeaconBlockRoot = make([]byte, fieldparams.RootLength)
	}
	if d.Target == nil {
		d.Target = &qrysmpb.Checkpoint{}
	}
	if d.Target.Root == nil {
		d.Target.Root = make([]byte, fieldparams.RootLength)
	}
	if d.Source == nil {
		d.Source = &qrysmpb.Checkpoint{}
	}
	if d.Source.Root == nil {
		d.Source.Root = make([]byte, fieldparams.RootLength)
	}
	return d
}

// HydrateV1AttestationData hydrates a v1 attestation data object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1AttestationData(d *qrlpb.AttestationData) *qrlpb.AttestationData {
	if d.BeaconBlockRoot == nil {
		d.BeaconBlockRoot = make([]byte, fieldparams.RootLength)
	}
	if d.Target == nil {
		d.Target = &qrlpb.Checkpoint{}
	}
	if d.Target.Root == nil {
		d.Target.Root = make([]byte, fieldparams.RootLength)
	}
	if d.Source == nil {
		d.Source = &qrlpb.Checkpoint{}
	}
	if d.Source.Root == nil {
		d.Source.Root = make([]byte, fieldparams.RootLength)
	}
	return d
}

// HydrateIndexedAttestation hydrates an indexed attestation with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateIndexedAttestation(a *qrysmpb.IndexedAttestation) *qrysmpb.IndexedAttestation {
	if a.Signatures == nil {
		sig := make([]byte, 4627)
		a.Signatures = [][]byte{sig}
	}
	if a.Data == nil {
		a.Data = &qrysmpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}
