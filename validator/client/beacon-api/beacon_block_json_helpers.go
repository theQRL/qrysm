package beacon_api

import (
	"strconv"

	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func jsonifyTransactions(transactions [][]byte) []string {
	jsonTransactions := make([]string, len(transactions))
	for index, transaction := range transactions {
		jsonTransaction := hexutil.Encode(transaction)
		jsonTransactions[index] = jsonTransaction
	}
	return jsonTransactions
}

func jsonifyExecutionData(executionData *qrysmpb.ExecutionData) *apimiddleware.ExecutionDataJson {
	return &apimiddleware.ExecutionDataJson{
		BlockHash:    hexutil.Encode(executionData.BlockHash),
		DepositCount: uint64ToString(executionData.DepositCount),
		DepositRoot:  hexutil.Encode(executionData.DepositRoot),
	}
}

func jsonifyAttestations(attestations []*qrysmpb.Attestation) []*apimiddleware.AttestationJson {
	jsonAttestations := make([]*apimiddleware.AttestationJson, len(attestations))
	for index, attestation := range attestations {
		jsonAttestations[index] = jsonifyAttestation(attestation)
	}
	return jsonAttestations
}

func jsonifyAttesterSlashings(attesterSlashings []*qrysmpb.AttesterSlashing) []*apimiddleware.AttesterSlashingJson {
	jsonAttesterSlashings := make([]*apimiddleware.AttesterSlashingJson, len(attesterSlashings))
	for index, attesterSlashing := range attesterSlashings {
		jsonAttesterSlashing := &apimiddleware.AttesterSlashingJson{
			Attestation_1: jsonifyIndexedAttestation(attesterSlashing.Attestation_1),
			Attestation_2: jsonifyIndexedAttestation(attesterSlashing.Attestation_2),
		}
		jsonAttesterSlashings[index] = jsonAttesterSlashing
	}
	return jsonAttesterSlashings
}

func jsonifyDeposits(deposits []*qrysmpb.Deposit) []*apimiddleware.DepositJson {
	jsonDeposits := make([]*apimiddleware.DepositJson, len(deposits))
	for depositIndex, deposit := range deposits {
		proofs := make([]string, len(deposit.Proof))
		for proofIndex, proof := range deposit.Proof {
			proofs[proofIndex] = hexutil.Encode(proof)
		}

		jsonDeposit := &apimiddleware.DepositJson{
			Data: &apimiddleware.Deposit_DataJson{
				Amount:                uint64ToString(deposit.Data.Amount),
				PublicKey:             hexutil.Encode(deposit.Data.PublicKey),
				Signature:             hexutil.Encode(deposit.Data.Signature),
				WithdrawalCredentials: hexutil.Encode(deposit.Data.WithdrawalCredentials),
			},
			Proof: proofs,
		}
		jsonDeposits[depositIndex] = jsonDeposit
	}
	return jsonDeposits
}

func jsonifyProposerSlashings(proposerSlashings []*qrysmpb.ProposerSlashing) []*apimiddleware.ProposerSlashingJson {
	jsonProposerSlashings := make([]*apimiddleware.ProposerSlashingJson, len(proposerSlashings))
	for index, proposerSlashing := range proposerSlashings {
		jsonProposerSlashing := &apimiddleware.ProposerSlashingJson{
			Header_1: jsonifySignedBeaconBlockHeader(proposerSlashing.Header_1),
			Header_2: jsonifySignedBeaconBlockHeader(proposerSlashing.Header_2),
		}
		jsonProposerSlashings[index] = jsonProposerSlashing
	}
	return jsonProposerSlashings
}

// JsonifySignedVoluntaryExits converts an array of voluntary exit structs to a JSON hex string compatible format.
func JsonifySignedVoluntaryExits(voluntaryExits []*qrysmpb.SignedVoluntaryExit) []*apimiddleware.SignedVoluntaryExitJson {
	jsonSignedVoluntaryExits := make([]*apimiddleware.SignedVoluntaryExitJson, len(voluntaryExits))
	for index, signedVoluntaryExit := range voluntaryExits {
		jsonSignedVoluntaryExit := &apimiddleware.SignedVoluntaryExitJson{
			Exit: &apimiddleware.VoluntaryExitJson{
				Epoch:          uint64ToString(signedVoluntaryExit.Exit.Epoch),
				ValidatorIndex: uint64ToString(signedVoluntaryExit.Exit.ValidatorIndex),
			},
			Signature: hexutil.Encode(signedVoluntaryExit.Signature),
		}
		jsonSignedVoluntaryExits[index] = jsonSignedVoluntaryExit
	}
	return jsonSignedVoluntaryExits
}

// JsonifySyncAggregate converts a sync aggregate struct to a JSON hex string compatible format.
func JsonifySyncAggregate(syncAggregate *qrysmpb.SyncAggregate) *apimiddleware.SyncAggregateJson {
	syncCommitteeSigs := make([]string, len(syncAggregate.SyncCommitteeSignatures))
	for i, sig := range syncAggregate.SyncCommitteeSignatures {
		syncCommitteeSigs[i] = hexutil.Encode(sig)
	}
	return &apimiddleware.SyncAggregateJson{
		SyncCommitteeBits:       hexutil.Encode(syncAggregate.SyncCommitteeBits),
		SyncCommitteeSignatures: syncCommitteeSigs,
	}
}

func jsonifySignedBeaconBlockHeader(signedBeaconBlockHeader *qrysmpb.SignedBeaconBlockHeader) *apimiddleware.SignedBeaconBlockHeaderJson {
	return &apimiddleware.SignedBeaconBlockHeaderJson{
		Header: &apimiddleware.BeaconBlockHeaderJson{
			BodyRoot:      hexutil.Encode(signedBeaconBlockHeader.Header.BodyRoot),
			ParentRoot:    hexutil.Encode(signedBeaconBlockHeader.Header.ParentRoot),
			ProposerIndex: uint64ToString(signedBeaconBlockHeader.Header.ProposerIndex),
			Slot:          uint64ToString(signedBeaconBlockHeader.Header.Slot),
			StateRoot:     hexutil.Encode(signedBeaconBlockHeader.Header.StateRoot),
		},
		Signature: hexutil.Encode(signedBeaconBlockHeader.Signature),
	}
}

func jsonifyIndexedAttestation(indexedAttestation *qrysmpb.IndexedAttestation) *apimiddleware.IndexedAttestationJson {
	attestingIndices := make([]string, len(indexedAttestation.AttestingIndices))
	for index, attestingIndex := range indexedAttestation.AttestingIndices {
		attestingIndex := uint64ToString(attestingIndex)
		attestingIndices[index] = attestingIndex
	}

	signatures := make([]string, len(indexedAttestation.Signatures))
	for i, sig := range indexedAttestation.Signatures {
		signatures[i] = hexutil.Encode(sig)
	}

	return &apimiddleware.IndexedAttestationJson{
		AttestingIndices: attestingIndices,
		Data:             jsonifyAttestationData(indexedAttestation.Data),
		Signatures:       signatures,
	}
}

func jsonifyAttestationData(attestationData *qrysmpb.AttestationData) *apimiddleware.AttestationDataJson {
	return &apimiddleware.AttestationDataJson{
		BeaconBlockRoot: hexutil.Encode(attestationData.BeaconBlockRoot),
		CommitteeIndex:  uint64ToString(attestationData.CommitteeIndex),
		Slot:            uint64ToString(attestationData.Slot),
		Source: &apimiddleware.CheckpointJson{
			Epoch: uint64ToString(attestationData.Source.Epoch),
			Root:  hexutil.Encode(attestationData.Source.Root),
		},
		Target: &apimiddleware.CheckpointJson{
			Epoch: uint64ToString(attestationData.Target.Epoch),
			Root:  hexutil.Encode(attestationData.Target.Root),
		},
	}
}

func jsonifyAttestation(attestation *qrysmpb.Attestation) *apimiddleware.AttestationJson {
	signatures := make([]string, len(attestation.Signatures))
	for i, sig := range attestation.Signatures {
		signatures[i] = hexutil.Encode(sig)
	}

	return &apimiddleware.AttestationJson{
		AggregationBits: hexutil.Encode(attestation.AggregationBits),
		Data:            jsonifyAttestationData(attestation.Data),
		Signatures:      signatures,
	}
}

func jsonifySignedAggregateAndProof(signedAggregateAndProof *qrysmpb.SignedAggregateAttestationAndProof) *apimiddleware.SignedAggregateAttestationAndProofJson {
	return &apimiddleware.SignedAggregateAttestationAndProofJson{
		Message: &apimiddleware.AggregateAttestationAndProofJson{
			AggregatorIndex: uint64ToString(signedAggregateAndProof.Message.AggregatorIndex),
			Aggregate:       jsonifyAttestation(signedAggregateAndProof.Message.Aggregate),
			SelectionProof:  hexutil.Encode(signedAggregateAndProof.Message.SelectionProof),
		},
		Signature: hexutil.Encode(signedAggregateAndProof.Signature),
	}
}

func jsonifyWithdrawals(withdrawals []*enginev1.Withdrawal) []*apimiddleware.WithdrawalJson {
	jsonWithdrawals := make([]*apimiddleware.WithdrawalJson, len(withdrawals))
	for index, withdrawal := range withdrawals {
		jsonWithdrawals[index] = &apimiddleware.WithdrawalJson{
			WithdrawalIndex:  strconv.FormatUint(withdrawal.Index, 10),
			ValidatorIndex:   strconv.FormatUint(uint64(withdrawal.ValidatorIndex), 10),
			ExecutionAddress: hexutil.Encode(withdrawal.Address),
			Amount:           strconv.FormatUint(withdrawal.Amount, 10),
		}
	}
	return jsonWithdrawals
}
