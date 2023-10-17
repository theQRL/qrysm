// Package blocks contains block processing libraries according to
// the Ethereum beacon chain spec.
package blocks

import (
	"context"

	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	ethpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// NewGenesisBlock returns the canonical, genesis block for the beacon chain protocol.
func NewGenesisBlock(stateRoot []byte) *ethpb.SignedBeaconBlock {
	zeroHash := params.BeaconConfig().ZeroHash[:]
	block := &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			ParentRoot: zeroHash,
			StateRoot:  bytesutil.PadTo(stateRoot, 32),
			Body: &ethpb.BeaconBlockBody{
				RandaoReveal: make([]byte, dilithium2.CryptoBytes),
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot: make([]byte, 32),
					BlockHash:   make([]byte, 32),
				},
				Graffiti: make([]byte, 32),
			},
		},
		Signature: params.BeaconConfig().EmptyDilithiumSignature[:],
	}
	return block
}

var ErrUnrecognizedState = errors.New("unknown underlying type for state.BeaconState value")

func NewGenesisBlockForState(ctx context.Context, st state.BeaconState) (interfaces.ReadOnlySignedBeaconBlock, error) {
	root, err := st.HashTreeRoot(ctx)
	if err != nil {
		return nil, err
	}
	ps := st.ToProto()
	switch ps.(type) {
	case *ethpb.BeaconState:
		return blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{
			Block: &ethpb.BeaconBlock{
				ParentRoot: params.BeaconConfig().ZeroHash[:],
				StateRoot:  root[:],
				Body: &ethpb.BeaconBlockBody{
					RandaoReveal: make([]byte, dilithium2.CryptoBytes),
					Eth1Data: &ethpb.Eth1Data{
						DepositRoot: make([]byte, 32),
						BlockHash:   make([]byte, 32),
					},
					Graffiti: make([]byte, 32),
				},
			},
			Signature: params.BeaconConfig().EmptyDilithiumSignature[:],
		})
	case *ethpb.BeaconStateAltair:
		return blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockAltair{
			Block: &ethpb.BeaconBlockAltair{
				ParentRoot: params.BeaconConfig().ZeroHash[:],
				StateRoot:  root[:],
				Body: &ethpb.BeaconBlockBodyAltair{
					RandaoReveal: make([]byte, dilithium2.CryptoBytes),
					Eth1Data: &ethpb.Eth1Data{
						DepositRoot: make([]byte, 32),
						BlockHash:   make([]byte, 32),
					},
					Graffiti: make([]byte, 32),
					SyncAggregate: &ethpb.SyncAggregate{
						SyncCommitteeBits:      make([]byte, fieldparams.SyncCommitteeLength/8),
						SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
					},
				},
			},
			Signature: params.BeaconConfig().EmptyDilithiumSignature[:],
		})
	case *ethpb.BeaconStateBellatrix:
		return blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockBellatrix{
			Block: &ethpb.BeaconBlockBellatrix{
				ParentRoot: params.BeaconConfig().ZeroHash[:],
				StateRoot:  root[:],
				Body: &ethpb.BeaconBlockBodyBellatrix{
					RandaoReveal: make([]byte, dilithium2.CryptoBytes),
					Eth1Data: &ethpb.Eth1Data{
						DepositRoot: make([]byte, 32),
						BlockHash:   make([]byte, 32),
					},
					Graffiti: make([]byte, 32),
					SyncAggregate: &ethpb.SyncAggregate{
						SyncCommitteeBits:      make([]byte, fieldparams.SyncCommitteeLength/8),
						SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
					},
					ExecutionPayload: &enginev1.ExecutionPayload{
						ParentHash:    make([]byte, 32),
						FeeRecipient:  make([]byte, 20),
						StateRoot:     make([]byte, 32),
						ReceiptsRoot:  make([]byte, 32),
						LogsBloom:     make([]byte, 256),
						PrevRandao:    make([]byte, 32),
						BaseFeePerGas: make([]byte, 32),
						BlockHash:     make([]byte, 32),
						Transactions:  make([][]byte, 0),
					},
				},
			},
			Signature: params.BeaconConfig().EmptyDilithiumSignature[:],
		})
	case *ethpb.BeaconStateCapella:
		return blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				ParentRoot: params.BeaconConfig().ZeroHash[:],
				StateRoot:  root[:],
				Body: &ethpb.BeaconBlockBodyCapella{
					RandaoReveal: make([]byte, dilithium2.CryptoBytes),
					Eth1Data: &ethpb.Eth1Data{
						DepositRoot: make([]byte, 32),
						BlockHash:   make([]byte, 32),
					},
					Graffiti: make([]byte, 32),
					SyncAggregate: &ethpb.SyncAggregate{
						SyncCommitteeBits:      make([]byte, fieldparams.SyncCommitteeLength/8),
						SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
					},
					ExecutionPayload: &enginev1.ExecutionPayloadCapella{
						ParentHash:    make([]byte, 32),
						FeeRecipient:  make([]byte, 20),
						StateRoot:     make([]byte, 32),
						ReceiptsRoot:  make([]byte, 32),
						LogsBloom:     make([]byte, 256),
						PrevRandao:    make([]byte, 32),
						BaseFeePerGas: make([]byte, 32),
						BlockHash:     make([]byte, 32),
						Transactions:  make([][]byte, 0),
						Withdrawals:   make([]*enginev1.Withdrawal, 0),
					},
				},
			},
			Signature: params.BeaconConfig().EmptyDilithiumSignature[:],
		})
	default:
		return nil, ErrUnrecognizedState
	}
}
