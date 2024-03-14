package migration

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

var (
	slot             = primitives.Slot(1)
	epoch            = primitives.Epoch(1)
	validatorIndex   = primitives.ValidatorIndex(1)
	committeeIndex   = primitives.CommitteeIndex(1)
	depositCount     = uint64(2)
	attestingIndices = []uint64{1, 2}
	blockNumber      = uint64(10)
	gasLimit         = uint64(10)
	gasUsed          = uint64(10)
	timestamp        = uint64(10)
	parentRoot       = bytesutil.PadTo([]byte("parentroot"), fieldparams.RootLength)
	stateRoot        = bytesutil.PadTo([]byte("stateroot"), fieldparams.RootLength)
	signature        = bytesutil.PadTo([]byte("signature"), 4595)
	signatures       = [][]byte{signature}
	randaoReveal     = bytesutil.PadTo([]byte("randaoreveal"), 4595)
	depositRoot      = bytesutil.PadTo([]byte("depositroot"), fieldparams.RootLength)
	blockHash        = bytesutil.PadTo([]byte("blockhash"), 32)
	beaconBlockRoot  = bytesutil.PadTo([]byte("beaconblockroot"), fieldparams.RootLength)
	sourceRoot       = bytesutil.PadTo([]byte("sourceroot"), fieldparams.RootLength)
	targetRoot       = bytesutil.PadTo([]byte("targetroot"), fieldparams.RootLength)
	bodyRoot         = bytesutil.PadTo([]byte("bodyroot"), fieldparams.RootLength)
	selectionProof   = bytesutil.PadTo([]byte("selectionproof"), 4595)
	parentHash       = bytesutil.PadTo([]byte("parenthash"), 32)
	feeRecipient     = bytesutil.PadTo([]byte("feerecipient"), 20)
	receiptsRoot     = bytesutil.PadTo([]byte("receiptsroot"), 32)
	logsBloom        = bytesutil.PadTo([]byte("logsbloom"), 256)
	prevRandao       = bytesutil.PadTo([]byte("prevrandao"), 32)
	extraData        = bytesutil.PadTo([]byte("extradata"), 32)
	baseFeePerGas    = bytesutil.PadTo([]byte("basefeepergas"), 32)
	transactionsRoot = bytesutil.PadTo([]byte("transactions"), 32)
	withdrawalsRoot  = bytesutil.PadTo([]byte("withdrawals"), 32)
	aggregationBits  = bitfield.Bitlist{0x01}
)

func Test_V1Alpha1AggregateAttAndProofToV1(t *testing.T) {
	proof := [32]byte{1}
	att := util.HydrateAttestation(&zondpbalpha.Attestation{
		Data: &zondpbalpha.AttestationData{
			Slot: 5,
		},
	})
	alpha := &zondpbalpha.AggregateAttestationAndProof{
		AggregatorIndex: 1,
		Aggregate:       att,
		SelectionProof:  proof[:],
	}
	v1 := V1Alpha1AggregateAttAndProofToV1(alpha)
	assert.Equal(t, v1.AggregatorIndex, primitives.ValidatorIndex(1))
	assert.DeepSSZEqual(t, v1.Aggregate.Data.Slot, att.Data.Slot)
	assert.DeepEqual(t, v1.SelectionProof, proof[:])
}

func Test_V1Alpha1AttSlashingToV1(t *testing.T) {
	alphaAttestation := &zondpbalpha.IndexedAttestation{
		AttestingIndices: attestingIndices,
		Data: &zondpbalpha.AttestationData{
			Slot:            slot,
			CommitteeIndex:  committeeIndex,
			BeaconBlockRoot: beaconBlockRoot,
			Source: &zondpbalpha.Checkpoint{
				Epoch: epoch,
				Root:  sourceRoot,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: epoch,
				Root:  targetRoot,
			},
		},
		Signatures: [][]byte{signature},
	}
	alphaSlashing := &zondpbalpha.AttesterSlashing{
		Attestation_1: alphaAttestation,
		Attestation_2: alphaAttestation,
	}

	v1Slashing := V1Alpha1AttSlashingToV1(alphaSlashing)
	alphaRoot, err := alphaSlashing.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Slashing.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, alphaRoot, v1Root)
}

func Test_V1Alpha1ProposerSlashingToV1(t *testing.T) {
	alphaHeader := util.HydrateSignedBeaconHeader(&zondpbalpha.SignedBeaconBlockHeader{})
	alphaHeader.Header.Slot = slot
	alphaHeader.Header.ProposerIndex = validatorIndex
	alphaHeader.Header.ParentRoot = parentRoot
	alphaHeader.Header.StateRoot = stateRoot
	alphaHeader.Header.BodyRoot = bodyRoot
	alphaHeader.Signature = signature
	alphaSlashing := &zondpbalpha.ProposerSlashing{
		Header_1: alphaHeader,
		Header_2: alphaHeader,
	}

	v1Slashing := V1Alpha1ProposerSlashingToV1(alphaSlashing)
	alphaRoot, err := alphaSlashing.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Slashing.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, alphaRoot, v1Root)
}

func Test_V1Alpha1ExitToV1(t *testing.T) {
	alphaExit := &zondpbalpha.SignedVoluntaryExit{
		Exit: &zondpbalpha.VoluntaryExit{
			Epoch:          epoch,
			ValidatorIndex: validatorIndex,
		},
		Signature: signature,
	}

	v1Exit := V1Alpha1ExitToV1(alphaExit)
	alphaRoot, err := alphaExit.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Exit.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, alphaRoot, v1Root)
}

func Test_V1ExitToV1Alpha1(t *testing.T) {
	v1Exit := &zondpbv1.SignedVoluntaryExit{
		Message: &zondpbv1.VoluntaryExit{
			Epoch:          epoch,
			ValidatorIndex: validatorIndex,
		},
		Signature: signature,
	}

	alphaExit := V1ExitToV1Alpha1(v1Exit)
	alphaRoot, err := alphaExit.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Exit.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, alphaRoot, v1Root)
}

func Test_V1AttSlashingToV1Alpha1(t *testing.T) {
	v1Attestation := &zondpbv1.IndexedAttestation{
		AttestingIndices: attestingIndices,
		Data: &zondpbv1.AttestationData{
			Slot:            slot,
			Index:           committeeIndex,
			BeaconBlockRoot: beaconBlockRoot,
			Source: &zondpbv1.Checkpoint{
				Epoch: epoch,
				Root:  sourceRoot,
			},
			Target: &zondpbv1.Checkpoint{
				Epoch: epoch,
				Root:  targetRoot,
			},
		},
		Signatures: [][]byte{signature},
	}
	v1Slashing := &zondpbv1.AttesterSlashing{
		Attestation_1: v1Attestation,
		Attestation_2: v1Attestation,
	}

	alphaSlashing := V1AttSlashingToV1Alpha1(v1Slashing)
	alphaRoot, err := alphaSlashing.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Slashing.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, alphaRoot)
}

func Test_V1ProposerSlashingToV1Alpha1(t *testing.T) {
	v1Header := &zondpbv1.SignedBeaconBlockHeader{
		Message: &zondpbv1.BeaconBlockHeader{
			Slot:          slot,
			ProposerIndex: validatorIndex,
			ParentRoot:    parentRoot,
			StateRoot:     stateRoot,
			BodyRoot:      bodyRoot,
		},
		Signature: signature,
	}
	v1Slashing := &zondpbv1.ProposerSlashing{
		SignedHeader_1: v1Header,
		SignedHeader_2: v1Header,
	}

	alphaSlashing := V1ProposerSlashingToV1Alpha1(v1Slashing)
	alphaRoot, err := alphaSlashing.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Slashing.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, alphaRoot, v1Root)
}

func Test_V1Alpha1AttToV1(t *testing.T) {
	alphaAtt := &zondpbalpha.Attestation{
		AggregationBits: aggregationBits,
		Data: &zondpbalpha.AttestationData{
			Slot:            slot,
			CommitteeIndex:  committeeIndex,
			BeaconBlockRoot: beaconBlockRoot,
			Source: &zondpbalpha.Checkpoint{
				Epoch: epoch,
				Root:  sourceRoot,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: epoch,
				Root:  targetRoot,
			},
		},
		Signatures: [][]byte{signature},
	}

	v1Att := V1Alpha1AttestationToV1(alphaAtt)
	v1Root, err := v1Att.HashTreeRoot()
	require.NoError(t, err)
	alphaRoot, err := alphaAtt.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, alphaRoot)
}

func Test_V1AttToV1Alpha1(t *testing.T) {
	v1Att := &zondpbv1.Attestation{
		AggregationBits: aggregationBits,
		Data: &zondpbv1.AttestationData{
			Slot:            slot,
			Index:           committeeIndex,
			BeaconBlockRoot: beaconBlockRoot,
			Source: &zondpbv1.Checkpoint{
				Epoch: epoch,
				Root:  sourceRoot,
			},
			Target: &zondpbv1.Checkpoint{
				Epoch: epoch,
				Root:  targetRoot,
			},
		},
		Signatures: [][]byte{signature},
	}

	alphaAtt := V1AttToV1Alpha1(v1Att)
	alphaRoot, err := alphaAtt.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Att.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, alphaRoot)
}

func Test_V1Alpha1ValidatorToV1(t *testing.T) {
	v1Alpha1Validator := &zondpbalpha.Validator{
		PublicKey:                  []byte("pubkey"),
		WithdrawalCredentials:      []byte("withdraw"),
		EffectiveBalance:           99,
		Slashed:                    true,
		ActivationEligibilityEpoch: 1,
		ActivationEpoch:            11,
		ExitEpoch:                  111,
		WithdrawableEpoch:          1111,
	}

	v1Validator := V1Alpha1ValidatorToV1(v1Alpha1Validator)
	require.NotNil(t, v1Validator)
	assert.DeepEqual(t, []byte("pubkey"), v1Validator.Pubkey)
	assert.DeepEqual(t, []byte("withdraw"), v1Validator.WithdrawalCredentials)
	assert.Equal(t, uint64(99), v1Validator.EffectiveBalance)
	assert.Equal(t, true, v1Validator.Slashed)
	assert.Equal(t, primitives.Epoch(1), v1Validator.ActivationEligibilityEpoch)
	assert.Equal(t, primitives.Epoch(11), v1Validator.ActivationEpoch)
	assert.Equal(t, primitives.Epoch(111), v1Validator.ExitEpoch)
	assert.Equal(t, primitives.Epoch(1111), v1Validator.WithdrawableEpoch)
}

func Test_V1ValidatorToV1Alpha1(t *testing.T) {
	v1Validator := &zondpbv1.Validator{
		Pubkey:                     []byte("pubkey"),
		WithdrawalCredentials:      []byte("withdraw"),
		EffectiveBalance:           99,
		Slashed:                    true,
		ActivationEligibilityEpoch: 1,
		ActivationEpoch:            11,
		ExitEpoch:                  111,
		WithdrawableEpoch:          1111,
	}

	v1Alpha1Validator := V1ValidatorToV1Alpha1(v1Validator)
	require.NotNil(t, v1Alpha1Validator)
	assert.DeepEqual(t, []byte("pubkey"), v1Alpha1Validator.PublicKey)
	assert.DeepEqual(t, []byte("withdraw"), v1Alpha1Validator.WithdrawalCredentials)
	assert.Equal(t, uint64(99), v1Alpha1Validator.EffectiveBalance)
	assert.Equal(t, true, v1Alpha1Validator.Slashed)
	assert.Equal(t, primitives.Epoch(1), v1Alpha1Validator.ActivationEligibilityEpoch)
	assert.Equal(t, primitives.Epoch(11), v1Alpha1Validator.ActivationEpoch)
	assert.Equal(t, primitives.Epoch(111), v1Alpha1Validator.ExitEpoch)
	assert.Equal(t, primitives.Epoch(1111), v1Alpha1Validator.WithdrawableEpoch)
}

func Test_V1SignedAggregateAttAndProofToV1Alpha1(t *testing.T) {
	v1Att := &zondpbv1.SignedAggregateAttestationAndProof{
		Message: &zondpbv1.AggregateAttestationAndProof{
			AggregatorIndex: 1,
			Aggregate:       util.HydrateV1Attestation(&zondpbv1.Attestation{}),
			SelectionProof:  selectionProof,
		},
		Signature: signature,
	}
	v1Alpha1Att := V1SignedAggregateAttAndProofToV1Alpha1(v1Att)

	v1Root, err := v1Att.HashTreeRoot()
	require.NoError(t, err)
	v1Alpha1Root, err := v1Alpha1Att.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, v1Alpha1Root)
}

func Test_V1AttestationToV1Alpha1(t *testing.T) {
	v1Att := util.HydrateV1Attestation(&zondpbv1.Attestation{})
	v1Alpha1Att := V1AttToV1Alpha1(v1Att)

	v1Root, err := v1Att.HashTreeRoot()
	require.NoError(t, err)
	v1Alpha1Root, err := v1Alpha1Att.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, v1Alpha1Root)
}

func TestV1Alpha1SignedContributionAndProofToV1(t *testing.T) {
	alphaContribution := &zondpbalpha.SignedContributionAndProof{
		Message: &zondpbalpha.ContributionAndProof{
			AggregatorIndex: validatorIndex,
			Contribution: &zondpbalpha.SyncCommitteeContribution{
				Slot:              slot,
				BlockRoot:         blockHash,
				SubcommitteeIndex: 1,
				AggregationBits:   bitfield.NewBitvector16(),
				Signatures:        signatures,
			},
			SelectionProof: signature,
		},
		Signature: signature,
	}
	v1Contribution := V1Alpha1SignedContributionAndProofToV1(alphaContribution)
	require.NotNil(t, v1Contribution)
	require.NotNil(t, v1Contribution.Message)
	require.NotNil(t, v1Contribution.Message.Contribution)
	assert.DeepEqual(t, signature, v1Contribution.Signature)
	msg := v1Contribution.Message
	assert.Equal(t, validatorIndex, msg.AggregatorIndex)
	assert.DeepEqual(t, signature, msg.SelectionProof)
	contrib := msg.Contribution
	assert.Equal(t, slot, contrib.Slot)
	assert.DeepEqual(t, blockHash, contrib.BeaconBlockRoot)
	assert.Equal(t, uint64(1), contrib.SubcommitteeIndex)
	assert.DeepEqual(t, bitfield.NewBitvector16(), contrib.AggregationBits)
	assert.DeepEqual(t, signatures, contrib.Signatures)
}

func Test_V1Alpha1BeaconBlockCapellaToV1Blinded(t *testing.T) {
	alphaBlock := util.HydrateBeaconBlockCapella(&zondpbalpha.BeaconBlockCapella{})
	alphaBlock.Slot = slot
	alphaBlock.ProposerIndex = validatorIndex
	alphaBlock.ParentRoot = parentRoot
	alphaBlock.StateRoot = stateRoot
	alphaBlock.Body.RandaoReveal = randaoReveal
	alphaBlock.Body.Eth1Data = &zondpbalpha.Eth1Data{
		DepositRoot:  depositRoot,
		DepositCount: depositCount,
		BlockHash:    blockHash,
	}
	syncCommitteeBits := bitfield.NewBitvector16()
	syncCommitteeBits.SetBitAt(100, true)
	alphaBlock.Body.SyncAggregate = &zondpbalpha.SyncAggregate{
		SyncCommitteeBits:       syncCommitteeBits,
		SyncCommitteeSignatures: [][]byte{signature},
	}
	alphaBlock.Body.ExecutionPayload.Transactions = [][]byte{[]byte("transaction1"), []byte("transaction2")}

	v1Block, err := V1Alpha1BeaconBlockCapellaToV1Blinded(alphaBlock)
	require.NoError(t, err)
	alphaRoot, err := alphaBlock.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, alphaRoot, v1Root)
}

func TestBeaconStateCapellaToProto(t *testing.T) {
	source, err := util.NewBeaconStateCapella(util.FillRootsNaturalOptCapella, func(state *zondpbalpha.BeaconStateCapella) error {
		state.GenesisTime = 1
		state.GenesisValidatorsRoot = bytesutil.PadTo([]byte("genesisvalidatorsroot"), 32)
		state.Slot = 2
		state.Fork = &zondpbalpha.Fork{
			PreviousVersion: bytesutil.PadTo([]byte("123"), 4),
			CurrentVersion:  bytesutil.PadTo([]byte("456"), 4),
			Epoch:           3,
		}
		state.LatestBlockHeader = &zondpbalpha.BeaconBlockHeader{
			Slot:          4,
			ProposerIndex: 5,
			ParentRoot:    bytesutil.PadTo([]byte("lbhparentroot"), 32),
			StateRoot:     bytesutil.PadTo([]byte("lbhstateroot"), 32),
			BodyRoot:      bytesutil.PadTo([]byte("lbhbodyroot"), 32),
		}
		state.BlockRoots = [][]byte{bytesutil.PadTo([]byte("blockroots"), 32)}
		state.StateRoots = [][]byte{bytesutil.PadTo([]byte("stateroots"), 32)}
		state.HistoricalRoots = [][]byte{bytesutil.PadTo([]byte("historicalroots"), 32)}
		state.Eth1Data = &zondpbalpha.Eth1Data{
			DepositRoot:  bytesutil.PadTo([]byte("e1ddepositroot"), 32),
			DepositCount: 6,
			BlockHash:    bytesutil.PadTo([]byte("e1dblockhash"), 32),
		}
		state.Eth1DataVotes = []*zondpbalpha.Eth1Data{{
			DepositRoot:  bytesutil.PadTo([]byte("e1dvdepositroot"), 32),
			DepositCount: 7,
			BlockHash:    bytesutil.PadTo([]byte("e1dvblockhash"), 32),
		}}
		state.Eth1DepositIndex = 8
		state.Validators = []*zondpbalpha.Validator{{
			PublicKey:                  bytesutil.PadTo([]byte("publickey"), 2592),
			WithdrawalCredentials:      bytesutil.PadTo([]byte("withdrawalcredentials"), 32),
			EffectiveBalance:           9,
			Slashed:                    true,
			ActivationEligibilityEpoch: 10,
			ActivationEpoch:            11,
			ExitEpoch:                  12,
			WithdrawableEpoch:          13,
		}}
		state.Balances = []uint64{14}
		state.RandaoMixes = [][]byte{bytesutil.PadTo([]byte("randaomixes"), 32)}
		state.Slashings = []uint64{15}
		state.JustificationBits = bitfield.Bitvector4{1}
		state.PreviousJustifiedCheckpoint = &zondpbalpha.Checkpoint{
			Epoch: 30,
			Root:  bytesutil.PadTo([]byte("pjcroot"), 32),
		}
		state.CurrentJustifiedCheckpoint = &zondpbalpha.Checkpoint{
			Epoch: 31,
			Root:  bytesutil.PadTo([]byte("cjcroot"), 32),
		}
		state.FinalizedCheckpoint = &zondpbalpha.Checkpoint{
			Epoch: 32,
			Root:  bytesutil.PadTo([]byte("fcroot"), 32),
		}
		state.PreviousEpochParticipation = []byte("previousepochparticipation")
		state.CurrentEpochParticipation = []byte("currentepochparticipation")
		state.InactivityScores = []uint64{1, 2, 3}
		state.CurrentSyncCommittee = &zondpbalpha.SyncCommittee{
			Pubkeys: [][]byte{bytesutil.PadTo([]byte("cscpubkeys"), 2592)},
		}
		state.NextSyncCommittee = &zondpbalpha.SyncCommittee{
			Pubkeys: [][]byte{bytesutil.PadTo([]byte("nscpubkeys"), 2592)},
		}
		state.LatestExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderCapella{
			ParentHash:       bytesutil.PadTo([]byte("parenthash"), 32),
			FeeRecipient:     bytesutil.PadTo([]byte("feerecipient"), 20),
			StateRoot:        bytesutil.PadTo([]byte("stateroot"), 32),
			ReceiptsRoot:     bytesutil.PadTo([]byte("receiptroot"), 32),
			LogsBloom:        bytesutil.PadTo([]byte("logsbloom"), 256),
			PrevRandao:       bytesutil.PadTo([]byte("prevrandao"), 32),
			BlockNumber:      123,
			GasLimit:         456,
			GasUsed:          789,
			Timestamp:        012,
			ExtraData:        []byte("extradata"),
			BaseFeePerGas:    bytesutil.PadTo([]byte("basefeepergas"), 32),
			BlockHash:        bytesutil.PadTo([]byte("blockhash"), 32),
			TransactionsRoot: bytesutil.PadTo([]byte("transactionsroot"), 32),
			WithdrawalsRoot:  bytesutil.PadTo([]byte("withdrawalsroot"), 32),
		}
		state.NextWithdrawalIndex = 123
		state.NextWithdrawalValidatorIndex = 123
		state.HistoricalSummaries = []*zondpbalpha.HistoricalSummary{
			{
				BlockSummaryRoot: bytesutil.PadTo([]byte("blocksummaryroot"), 32),
				StateSummaryRoot: bytesutil.PadTo([]byte("statesummaryroot"), 32),
			},
			{
				BlockSummaryRoot: bytesutil.PadTo([]byte("blocksummaryroot2"), 32),
				StateSummaryRoot: bytesutil.PadTo([]byte("statesummaryroot2"), 32),
			}}
		return nil
	})
	require.NoError(t, err)

	result, err := BeaconStateCapellaToProto(source)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint64(1), result.GenesisTime)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("genesisvalidatorsroot"), 32), result.GenesisValidatorsRoot)
	assert.Equal(t, primitives.Slot(2), result.Slot)
	resultFork := result.Fork
	require.NotNil(t, resultFork)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("123"), 4), resultFork.PreviousVersion)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("456"), 4), resultFork.CurrentVersion)
	assert.Equal(t, primitives.Epoch(3), resultFork.Epoch)
	resultLatestBlockHeader := result.LatestBlockHeader
	require.NotNil(t, resultLatestBlockHeader)
	assert.Equal(t, primitives.Slot(4), resultLatestBlockHeader.Slot)
	assert.Equal(t, primitives.ValidatorIndex(5), resultLatestBlockHeader.ProposerIndex)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("lbhparentroot"), 32), resultLatestBlockHeader.ParentRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("lbhstateroot"), 32), resultLatestBlockHeader.StateRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("lbhbodyroot"), 32), resultLatestBlockHeader.BodyRoot)
	assert.Equal(t, 1024, len(result.BlockRoots))
	assert.DeepEqual(t, bytesutil.PadTo([]byte("blockroots"), 32), result.BlockRoots[0])
	assert.Equal(t, 1024, len(result.StateRoots))
	assert.DeepEqual(t, bytesutil.PadTo([]byte("stateroots"), 32), result.StateRoots[0])
	assert.Equal(t, 1, len(result.HistoricalRoots))
	assert.DeepEqual(t, bytesutil.PadTo([]byte("historicalroots"), 32), result.HistoricalRoots[0])
	resultEth1Data := result.Eth1Data
	require.NotNil(t, resultEth1Data)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("e1ddepositroot"), 32), resultEth1Data.DepositRoot)
	assert.Equal(t, uint64(6), resultEth1Data.DepositCount)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("e1dblockhash"), 32), resultEth1Data.BlockHash)
	require.Equal(t, 1, len(result.Eth1DataVotes))
	resultEth1DataVote := result.Eth1DataVotes[0]
	require.NotNil(t, resultEth1DataVote)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("e1dvdepositroot"), 32), resultEth1DataVote.DepositRoot)
	assert.Equal(t, uint64(7), resultEth1DataVote.DepositCount)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("e1dvblockhash"), 32), resultEth1DataVote.BlockHash)
	assert.Equal(t, uint64(8), result.Eth1DepositIndex)
	require.Equal(t, 1, len(result.Validators))
	resultValidator := result.Validators[0]
	require.NotNil(t, resultValidator)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("publickey"), 2592), resultValidator.Pubkey)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("withdrawalcredentials"), 32), resultValidator.WithdrawalCredentials)
	assert.Equal(t, uint64(9), resultValidator.EffectiveBalance)
	assert.Equal(t, true, resultValidator.Slashed)
	assert.Equal(t, primitives.Epoch(10), resultValidator.ActivationEligibilityEpoch)
	assert.Equal(t, primitives.Epoch(11), resultValidator.ActivationEpoch)
	assert.Equal(t, primitives.Epoch(12), resultValidator.ExitEpoch)
	assert.Equal(t, primitives.Epoch(13), resultValidator.WithdrawableEpoch)
	assert.DeepEqual(t, []uint64{14}, result.Balances)
	assert.Equal(t, 65536, len(result.RandaoMixes))
	assert.DeepEqual(t, bytesutil.PadTo([]byte("randaomixes"), 32), result.RandaoMixes[0])
	assert.DeepEqual(t, []uint64{15}, result.Slashings)
	assert.DeepEqual(t, bitfield.Bitvector4{1}, result.JustificationBits)
	resultPrevJustifiedCheckpoint := result.PreviousJustifiedCheckpoint
	require.NotNil(t, resultPrevJustifiedCheckpoint)
	assert.Equal(t, primitives.Epoch(30), resultPrevJustifiedCheckpoint.Epoch)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("pjcroot"), 32), resultPrevJustifiedCheckpoint.Root)
	resultCurrJustifiedCheckpoint := result.CurrentJustifiedCheckpoint
	require.NotNil(t, resultCurrJustifiedCheckpoint)
	assert.Equal(t, primitives.Epoch(31), resultCurrJustifiedCheckpoint.Epoch)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("cjcroot"), 32), resultCurrJustifiedCheckpoint.Root)
	resultFinalizedCheckpoint := result.FinalizedCheckpoint
	require.NotNil(t, resultFinalizedCheckpoint)
	assert.Equal(t, primitives.Epoch(32), resultFinalizedCheckpoint.Epoch)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("fcroot"), 32), resultFinalizedCheckpoint.Root)
	assert.DeepEqual(t, []byte("previousepochparticipation"), result.PreviousEpochParticipation)
	assert.DeepEqual(t, []byte("currentepochparticipation"), result.CurrentEpochParticipation)
	assert.DeepEqual(t, []uint64{1, 2, 3}, result.InactivityScores)
	require.NotNil(t, result.CurrentSyncCommittee)
	assert.DeepEqual(t, [][]byte{bytesutil.PadTo([]byte("cscpubkeys"), 2592)}, result.CurrentSyncCommittee.Pubkeys)
	require.NotNil(t, result.NextSyncCommittee)
	assert.DeepEqual(t, [][]byte{bytesutil.PadTo([]byte("nscpubkeys"), 2592)}, result.NextSyncCommittee.Pubkeys)
	resultLatestExecutionPayloadHeader := result.LatestExecutionPayloadHeader
	require.NotNil(t, resultLatestExecutionPayloadHeader)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("parenthash"), 32), resultLatestExecutionPayloadHeader.ParentHash)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("feerecipient"), 20), resultLatestExecutionPayloadHeader.FeeRecipient)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("stateroot"), 32), resultLatestExecutionPayloadHeader.StateRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("receiptroot"), 32), resultLatestExecutionPayloadHeader.ReceiptsRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("logsbloom"), 256), resultLatestExecutionPayloadHeader.LogsBloom)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("prevrandao"), 32), resultLatestExecutionPayloadHeader.PrevRandao)
	assert.Equal(t, uint64(123), resultLatestExecutionPayloadHeader.BlockNumber)
	assert.Equal(t, uint64(456), resultLatestExecutionPayloadHeader.GasLimit)
	assert.Equal(t, uint64(789), resultLatestExecutionPayloadHeader.GasUsed)
	assert.Equal(t, uint64(012), resultLatestExecutionPayloadHeader.Timestamp)
	assert.DeepEqual(t, []byte("extradata"), resultLatestExecutionPayloadHeader.ExtraData)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("basefeepergas"), 32), resultLatestExecutionPayloadHeader.BaseFeePerGas)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("blockhash"), 32), resultLatestExecutionPayloadHeader.BlockHash)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("transactionsroot"), 32), resultLatestExecutionPayloadHeader.TransactionsRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("withdrawalsroot"), 32), resultLatestExecutionPayloadHeader.WithdrawalsRoot)
	assert.Equal(t, uint64(123), result.NextWithdrawalIndex)
	assert.Equal(t, primitives.ValidatorIndex(123), result.NextWithdrawalValidatorIndex)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("blocksummaryroot"), 32), result.HistoricalSummaries[0].BlockSummaryRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("statesummaryroot"), 32), result.HistoricalSummaries[0].StateSummaryRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("blocksummaryroot2"), 32), result.HistoricalSummaries[1].BlockSummaryRoot)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("statesummaryroot2"), 32), result.HistoricalSummaries[1].StateSummaryRoot)
}

func TestV1Alpha1SignedDilithiumToExecChangeToV1(t *testing.T) {
	alphaChange := &zondpbalpha.SignedDilithiumToExecutionChange{
		Message: &zondpbalpha.DilithiumToExecutionChange{
			ValidatorIndex:      validatorIndex,
			FromDilithiumPubkey: bytesutil.PadTo([]byte("fromdilithiumpubkey"), 2592),
			ToExecutionAddress:  bytesutil.PadTo([]byte("toexecutionaddress"), 20),
		},
		Signature: signature,
	}
	change := V1Alpha1SignedDilithiumToExecChangeToV1(alphaChange)
	require.NotNil(t, change)
	require.NotNil(t, change.Message)
	assert.DeepEqual(t, signature, change.Signature)
	assert.Equal(t, validatorIndex, change.Message.ValidatorIndex)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("fromdilithiumpubkey"), 2592), change.Message.FromDilithiumPubkey)
	assert.DeepEqual(t, bytesutil.PadTo([]byte("toexecutionaddress"), 20), change.Message.ToExecutionAddress)
}
