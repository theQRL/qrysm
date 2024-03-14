package util

import (
	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	p2pType "github.com/theQRL/qrysm/v4/beacon-chain/p2p/types"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func generateSyncAggregate(st state.BeaconState, privs []dilithium.DilithiumKey, parentRoot [32]byte) (*zondpb.SyncAggregate, error) {
	nextSlotEpoch := slots.ToEpoch(st.Slot() + 1)
	currEpoch := slots.ToEpoch(st.Slot())

	var syncCommittee *zondpb.SyncCommittee
	var err error
	if slots.SyncCommitteePeriod(currEpoch) == slots.SyncCommitteePeriod(nextSlotEpoch) {
		syncCommittee, err = st.CurrentSyncCommittee()
		if err != nil {
			return nil, err
		}
	} else {
		syncCommittee, err = st.NextSyncCommittee()
		if err != nil {
			return nil, err
		}
	}
	sigs := make([][]byte, 0, len(syncCommittee.Pubkeys))
	var bVector []byte
	currSize := new(zondpb.SyncAggregate).SyncCommitteeBits.Len()
	switch currSize {
	case 512:
		bVector = bitfield.NewBitvector512()
	case 32:
		bVector = bitfield.NewBitvector32()
	case 16:
		bVector = bitfield.NewBitvector16()
	default:
		return nil, errors.New("invalid bit vector size")
	}

	for i, p := range syncCommittee.Pubkeys {
		idx, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes2592(p))
		if !ok {
			continue
		}
		d, err := signing.Domain(st.Fork(), slots.ToEpoch(st.Slot()), params.BeaconConfig().DomainSyncCommittee, st.GenesisValidatorsRoot())
		if err != nil {
			return nil, err
		}
		sszBytes := p2pType.SSZBytes(parentRoot[:])
		r, err := signing.ComputeSigningRoot(&sszBytes, d)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, privs[idx].Sign(r[:]).Marshal())
		if currSize == 512 {
			bitfield.Bitvector512(bVector).SetBitAt(uint64(i), true)
		}
		if currSize == 32 {
			bitfield.Bitvector32(bVector).SetBitAt(uint64(i), true)
		}
		if currSize == 16 {
			bitfield.Bitvector16(bVector).SetBitAt(uint64(i), true)
		}
	}

	return &zondpb.SyncAggregate{SyncCommitteeSignatures: sigs, SyncCommitteeBits: bVector}, nil
}
