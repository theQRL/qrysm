package qrl

import (
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
)

// CopyExecutionData copies the provided executionData object.
func CopyExecutionData(data *ExecutionData) *ExecutionData {
	if data == nil {
		return nil
	}
	return &ExecutionData{
		DepositRoot:  bytesutil.SafeCopyBytes(data.DepositRoot),
		DepositCount: data.DepositCount,
		BlockHash:    bytesutil.SafeCopyBytes(data.BlockHash),
	}
}

// CopyPendingAttestationSlice copies the provided slice of pending attestation objects.
func CopyPendingAttestationSlice(input []*PendingAttestation) []*PendingAttestation {
	if input == nil {
		return nil
	}

	res := make([]*PendingAttestation, len(input))
	for i := range res {
		res[i] = CopyPendingAttestation(input[i])
	}
	return res
}

// CopyPendingAttestation copies the provided pending attestation object.
func CopyPendingAttestation(att *PendingAttestation) *PendingAttestation {
	if att == nil {
		return nil
	}
	data := CopyAttestationData(att.Data)
	return &PendingAttestation{
		AggregationBits: bytesutil.SafeCopyBytes(att.AggregationBits),
		Data:            data,
		InclusionDelay:  att.InclusionDelay,
		ProposerIndex:   att.ProposerIndex,
	}
}

// CopyAttestation copies the provided attestation object.
func CopyAttestation(att *Attestation) *Attestation {
	if att == nil {
		return nil
	}

	signatures := make([][]byte, len(att.Signatures))
	for i, sig := range att.Signatures {
		signatures[i] = bytesutil.SafeCopyBytes(sig)
	}

	return &Attestation{
		AggregationBits: bytesutil.SafeCopyBytes(att.AggregationBits),
		Data:            CopyAttestationData(att.Data),
		Signatures:      signatures,
	}
}

// CopyAttestationData copies the provided AttestationData object.
func CopyAttestationData(attData *AttestationData) *AttestationData {
	if attData == nil {
		return nil
	}
	return &AttestationData{
		Slot:            attData.Slot,
		CommitteeIndex:  attData.CommitteeIndex,
		BeaconBlockRoot: bytesutil.SafeCopyBytes(attData.BeaconBlockRoot),
		Source:          CopyCheckpoint(attData.Source),
		Target:          CopyCheckpoint(attData.Target),
	}
}

// CopyCheckpoint copies the provided checkpoint.
func CopyCheckpoint(cp *Checkpoint) *Checkpoint {
	if cp == nil {
		return nil
	}
	return &Checkpoint{
		Epoch: cp.Epoch,
		Root:  bytesutil.SafeCopyBytes(cp.Root),
	}
}

// CopyProposerSlashings copies the provided ProposerSlashing array.
func CopyProposerSlashings(slashings []*ProposerSlashing) []*ProposerSlashing {
	if slashings == nil {
		return nil
	}
	newSlashings := make([]*ProposerSlashing, len(slashings))
	for i, att := range slashings {
		newSlashings[i] = CopyProposerSlashing(att)
	}
	return newSlashings
}

// CopyProposerSlashing copies the provided ProposerSlashing.
func CopyProposerSlashing(slashing *ProposerSlashing) *ProposerSlashing {
	if slashing == nil {
		return nil
	}
	return &ProposerSlashing{
		Header_1: CopySignedBeaconBlockHeader(slashing.Header_1),
		Header_2: CopySignedBeaconBlockHeader(slashing.Header_2),
	}
}

// CopySignedBeaconBlockHeader copies the provided SignedBeaconBlockHeader.
func CopySignedBeaconBlockHeader(header *SignedBeaconBlockHeader) *SignedBeaconBlockHeader {
	if header == nil {
		return nil
	}
	return &SignedBeaconBlockHeader{
		Header:    CopyBeaconBlockHeader(header.Header),
		Signature: bytesutil.SafeCopyBytes(header.Signature),
	}
}

// CopyBeaconBlockHeader copies the provided BeaconBlockHeader.
func CopyBeaconBlockHeader(header *BeaconBlockHeader) *BeaconBlockHeader {
	if header == nil {
		return nil
	}
	parentRoot := bytesutil.SafeCopyBytes(header.ParentRoot)
	stateRoot := bytesutil.SafeCopyBytes(header.StateRoot)
	bodyRoot := bytesutil.SafeCopyBytes(header.BodyRoot)
	return &BeaconBlockHeader{
		Slot:          header.Slot,
		ProposerIndex: header.ProposerIndex,
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		BodyRoot:      bodyRoot,
	}
}

// CopyAttesterSlashings copies the provided AttesterSlashings array.
func CopyAttesterSlashings(slashings []*AttesterSlashing) []*AttesterSlashing {
	if slashings == nil {
		return nil
	}
	newSlashings := make([]*AttesterSlashing, len(slashings))
	for i, slashing := range slashings {
		newSlashings[i] = &AttesterSlashing{
			Attestation_1: CopyIndexedAttestation(slashing.Attestation_1),
			Attestation_2: CopyIndexedAttestation(slashing.Attestation_2),
		}
	}
	return newSlashings
}

// CopyIndexedAttestation copies the provided IndexedAttestation.
func CopyIndexedAttestation(indexedAtt *IndexedAttestation) *IndexedAttestation {
	var (
		indices    []uint64
		signatures [][]byte
	)
	if indexedAtt == nil {
		return nil
	} else if indexedAtt.AttestingIndices != nil {
		indices = make([]uint64, len(indexedAtt.AttestingIndices))
		copy(indices, indexedAtt.AttestingIndices)
		signatures = make([][]byte, len(indexedAtt.Signatures))
		for i, sig := range indexedAtt.Signatures {
			signatures[i] = bytesutil.SafeCopyBytes(sig)
		}
	}
	return &IndexedAttestation{
		AttestingIndices: indices,
		Data:             CopyAttestationData(indexedAtt.Data),
		Signatures:       signatures,
	}
}

// CopyAttestations copies the provided Attestation array.
func CopyAttestations(attestations []*Attestation) []*Attestation {
	if attestations == nil {
		return nil
	}
	newAttestations := make([]*Attestation, len(attestations))
	for i, att := range attestations {
		newAttestations[i] = CopyAttestation(att)
	}
	return newAttestations
}

// CopyDeposits copies the provided deposit array.
func CopyDeposits(deposits []*Deposit) []*Deposit {
	if deposits == nil {
		return nil
	}
	newDeposits := make([]*Deposit, len(deposits))
	for i, dep := range deposits {
		newDeposits[i] = CopyDeposit(dep)
	}
	return newDeposits
}

// CopyDeposit copies the provided deposit.
func CopyDeposit(deposit *Deposit) *Deposit {
	if deposit == nil {
		return nil
	}
	return &Deposit{
		Proof: bytesutil.SafeCopy2dBytes(deposit.Proof),
		Data:  CopyDepositData(deposit.Data),
	}
}

// CopyDepositData copies the provided deposit data.
func CopyDepositData(depData *Deposit_Data) *Deposit_Data {
	if depData == nil {
		return nil
	}
	return &Deposit_Data{
		PublicKey:             bytesutil.SafeCopyBytes(depData.PublicKey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(depData.WithdrawalCredentials),
		Amount:                depData.Amount,
		Signature:             bytesutil.SafeCopyBytes(depData.Signature),
	}
}

// CopySignedVoluntaryExits copies the provided SignedVoluntaryExits array.
func CopySignedVoluntaryExits(exits []*SignedVoluntaryExit) []*SignedVoluntaryExit {
	if exits == nil {
		return nil
	}
	newExits := make([]*SignedVoluntaryExit, len(exits))
	for i, exit := range exits {
		newExits[i] = CopySignedVoluntaryExit(exit)
	}
	return newExits
}

// CopySignedVoluntaryExit copies the provided SignedVoluntaryExit.
func CopySignedVoluntaryExit(exit *SignedVoluntaryExit) *SignedVoluntaryExit {
	if exit == nil {
		return nil
	}
	return &SignedVoluntaryExit{
		Exit: &VoluntaryExit{
			Epoch:          exit.Exit.Epoch,
			ValidatorIndex: exit.Exit.ValidatorIndex,
		},
		Signature: bytesutil.SafeCopyBytes(exit.Signature),
	}
}

// CopyValidator copies the provided validator.
func CopyValidator(val *Validator) *Validator {
	pubKey := make([]byte, len(val.PublicKey))
	copy(pubKey, val.PublicKey)
	withdrawalCreds := make([]byte, len(val.WithdrawalCredentials))
	copy(withdrawalCreds, val.WithdrawalCredentials)
	return &Validator{
		PublicKey:                  pubKey,
		WithdrawalCredentials:      withdrawalCreds,
		EffectiveBalance:           val.EffectiveBalance,
		Slashed:                    val.Slashed,
		ActivationEligibilityEpoch: val.ActivationEligibilityEpoch,
		ActivationEpoch:            val.ActivationEpoch,
		ExitEpoch:                  val.ExitEpoch,
		WithdrawableEpoch:          val.WithdrawableEpoch,
	}
}

// CopySyncCommitteeMessage copies the provided sync committee message object.
func CopySyncCommitteeMessage(s *SyncCommitteeMessage) *SyncCommitteeMessage {
	if s == nil {
		return nil
	}
	return &SyncCommitteeMessage{
		Slot:           s.Slot,
		BlockRoot:      bytesutil.SafeCopyBytes(s.BlockRoot),
		ValidatorIndex: s.ValidatorIndex,
		Signature:      bytesutil.SafeCopyBytes(s.Signature),
	}
}

// CopySyncCommitteeContribution copies the provided sync committee contribution object.
func CopySyncCommitteeContribution(c *SyncCommitteeContribution) *SyncCommitteeContribution {
	if c == nil {
		return nil
	}

	signatures := make([][]byte, len(c.Signatures))
	for i, sig := range c.Signatures {
		signatures[i] = bytesutil.SafeCopyBytes(sig)
	}

	return &SyncCommitteeContribution{
		Slot:              c.Slot,
		BlockRoot:         bytesutil.SafeCopyBytes(c.BlockRoot),
		SubcommitteeIndex: c.SubcommitteeIndex,
		AggregationBits:   bytesutil.SafeCopyBytes(c.AggregationBits),
		Signatures:        signatures,
	}
}

// CopySyncAggregate copies the provided sync aggregate object.
func CopySyncAggregate(a *SyncAggregate) *SyncAggregate {
	if a == nil {
		return nil
	}

	signatures := make([][]byte, len(a.SyncCommitteeSignatures))
	for i, sig := range a.SyncCommitteeSignatures {
		signatures[i] = bytesutil.SafeCopyBytes(sig)
	}

	return &SyncAggregate{
		SyncCommitteeBits:       bytesutil.SafeCopyBytes(a.SyncCommitteeBits),
		SyncCommitteeSignatures: signatures,
	}
}

// CopySignedBeaconBlockZond copies the provided SignedBeaconBlockZond.
func CopySignedBeaconBlockZond(sigBlock *SignedBeaconBlockZond) *SignedBeaconBlockZond {
	if sigBlock == nil {
		return nil
	}
	return &SignedBeaconBlockZond{
		Block:     CopyBeaconBlockZond(sigBlock.Block),
		Signature: bytesutil.SafeCopyBytes(sigBlock.Signature),
	}
}

// CopyBeaconBlockZond copies the provided BeaconBlockZond.
func CopyBeaconBlockZond(block *BeaconBlockZond) *BeaconBlockZond {
	if block == nil {
		return nil
	}
	return &BeaconBlockZond{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(block.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(block.StateRoot),
		Body:          CopyBeaconBlockBodyZond(block.Body),
	}
}

// CopyBeaconBlockBodyZond copies the provided BeaconBlockBodyZond.
func CopyBeaconBlockBodyZond(body *BeaconBlockBodyZond) *BeaconBlockBodyZond {
	if body == nil {
		return nil
	}
	return &BeaconBlockBodyZond{
		RandaoReveal:      bytesutil.SafeCopyBytes(body.RandaoReveal),
		ExecutionData:     CopyExecutionData(body.ExecutionData),
		Graffiti:          bytesutil.SafeCopyBytes(body.Graffiti),
		ProposerSlashings: CopyProposerSlashings(body.ProposerSlashings),
		AttesterSlashings: CopyAttesterSlashings(body.AttesterSlashings),
		Attestations:      CopyAttestations(body.Attestations),
		Deposits:          CopyDeposits(body.Deposits),
		VoluntaryExits:    CopySignedVoluntaryExits(body.VoluntaryExits),
		SyncAggregate:     CopySyncAggregate(body.SyncAggregate),
		ExecutionPayload:  CopyExecutionPayloadZond(body.ExecutionPayload),
	}
}

// CopySignedBlindedBeaconBlockZond copies the provided SignedBlindedBeaconBlockZond.
func CopySignedBlindedBeaconBlockZond(sigBlock *SignedBlindedBeaconBlockZond) *SignedBlindedBeaconBlockZond {
	if sigBlock == nil {
		return nil
	}
	return &SignedBlindedBeaconBlockZond{
		Block:     CopyBlindedBeaconBlockZond(sigBlock.Block),
		Signature: bytesutil.SafeCopyBytes(sigBlock.Signature),
	}
}

// CopyBlindedBeaconBlockZond copies the provided BlindedBeaconBlockZond.
func CopyBlindedBeaconBlockZond(block *BlindedBeaconBlockZond) *BlindedBeaconBlockZond {
	if block == nil {
		return nil
	}
	return &BlindedBeaconBlockZond{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(block.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(block.StateRoot),
		Body:          CopyBlindedBeaconBlockBodyZond(block.Body),
	}
}

// CopyBlindedBeaconBlockBodyZond copies the provided BlindedBeaconBlockBodyZond.
func CopyBlindedBeaconBlockBodyZond(body *BlindedBeaconBlockBodyZond) *BlindedBeaconBlockBodyZond {
	if body == nil {
		return nil
	}
	return &BlindedBeaconBlockBodyZond{
		RandaoReveal:           bytesutil.SafeCopyBytes(body.RandaoReveal),
		ExecutionData:          CopyExecutionData(body.ExecutionData),
		Graffiti:               bytesutil.SafeCopyBytes(body.Graffiti),
		ProposerSlashings:      CopyProposerSlashings(body.ProposerSlashings),
		AttesterSlashings:      CopyAttesterSlashings(body.AttesterSlashings),
		Attestations:           CopyAttestations(body.Attestations),
		Deposits:               CopyDeposits(body.Deposits),
		VoluntaryExits:         CopySignedVoluntaryExits(body.VoluntaryExits),
		SyncAggregate:          CopySyncAggregate(body.SyncAggregate),
		ExecutionPayloadHeader: CopyExecutionPayloadHeaderZond(body.ExecutionPayloadHeader),
	}
}

// CopyExecutionPayloadZond copies the provided execution payload.
func CopyExecutionPayloadZond(payload *enginev1.ExecutionPayloadZond) *enginev1.ExecutionPayloadZond {
	if payload == nil {
		return nil
	}

	return &enginev1.ExecutionPayloadZond{
		ParentHash:    bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:  bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:     bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:  bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:     bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:    bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:   payload.BlockNumber,
		GasLimit:      payload.GasLimit,
		GasUsed:       payload.GasUsed,
		Timestamp:     payload.Timestamp,
		ExtraData:     bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas: bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:     bytesutil.SafeCopyBytes(payload.BlockHash),
		Transactions:  bytesutil.SafeCopy2dBytes(payload.Transactions),
		Withdrawals:   CopyWithdrawalSlice(payload.Withdrawals),
	}
}

// CopyExecutionPayloadHeaderZond copies the provided execution payload object.
func CopyExecutionPayloadHeaderZond(payload *enginev1.ExecutionPayloadHeaderZond) *enginev1.ExecutionPayloadHeaderZond {
	if payload == nil {
		return nil
	}
	return &enginev1.ExecutionPayloadHeaderZond{
		ParentHash:       bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:     bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:        bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:     bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:        bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:       bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:      payload.BlockNumber,
		GasLimit:         payload.GasLimit,
		GasUsed:          payload.GasUsed,
		Timestamp:        payload.Timestamp,
		BaseFeePerGas:    bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		ExtraData:        bytesutil.SafeCopyBytes(payload.ExtraData),
		BlockHash:        bytesutil.SafeCopyBytes(payload.BlockHash),
		TransactionsRoot: bytesutil.SafeCopyBytes(payload.TransactionsRoot),
		WithdrawalsRoot:  bytesutil.SafeCopyBytes(payload.WithdrawalsRoot),
	}
}

// CopyWithdrawalSlice copies the provided slice of withdrawals.
func CopyWithdrawalSlice(withdrawals []*enginev1.Withdrawal) []*enginev1.Withdrawal {
	if withdrawals == nil {
		return nil
	}

	res := make([]*enginev1.Withdrawal, len(withdrawals))
	for i := range res {
		res[i] = CopyWithdrawal(withdrawals[i])
	}
	return res
}

// CopyWithdrawal copies the provided withdrawal object.
func CopyWithdrawal(withdrawal *enginev1.Withdrawal) *enginev1.Withdrawal {
	if withdrawal == nil {
		return nil
	}

	return &enginev1.Withdrawal{
		Index:          withdrawal.Index,
		ValidatorIndex: withdrawal.ValidatorIndex,
		Address:        bytesutil.SafeCopyBytes(withdrawal.Address),
		Amount:         withdrawal.Amount,
	}
}

// CopyHistoricalSummaries copies the historical summaries.
func CopyHistoricalSummaries(summaries []*HistoricalSummary) []*HistoricalSummary {
	if summaries == nil {
		return nil
	}
	newSummaries := make([]*HistoricalSummary, len(summaries))
	for i, s := range summaries {
		newSummaries[i] = &HistoricalSummary{
			BlockSummaryRoot: bytesutil.SafeCopyBytes(s.BlockSummaryRoot),
			StateSummaryRoot: bytesutil.SafeCopyBytes(s.StateSummaryRoot),
		}
	}
	return newSummaries
}
