package shared

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/consensus-types/validator"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type Attestation struct {
	AggregationBits string           `json:"aggregation_bits"`
	Data            *AttestationData `json:"data"`
	Signatures      []string         `json:"signatures"`
}

type AttestationData struct {
	Slot            string      `json:"slot"`
	CommitteeIndex  string      `json:"index"`
	BeaconBlockRoot string      `json:"beacon_block_root"`
	Source          *Checkpoint `json:"source"`
	Target          *Checkpoint `json:"target"`
}

type Checkpoint struct {
	Epoch string `json:"epoch"`
	Root  string `json:"root"`
}

type Committee struct {
	Index      string   `json:"index"`
	Slot       string   `json:"slot"`
	Validators []string `json:"validators"`
}

type SignedContributionAndProof struct {
	Message   *ContributionAndProof `json:"message"`
	Signature string                `json:"signature"`
}

type ContributionAndProof struct {
	AggregatorIndex string                     `json:"aggregator_index"`
	Contribution    *SyncCommitteeContribution `json:"contribution"`
	SelectionProof  string                     `json:"selection_proof"`
}

type SyncCommitteeContribution struct {
	Slot              string   `json:"slot"`
	BeaconBlockRoot   string   `json:"beacon_block_root"`
	SubcommitteeIndex string   `json:"subcommittee_index"`
	AggregationBits   string   `json:"aggregation_bits"`
	Signatures        []string `json:"signatures"`
}

type SignedAggregateAttestationAndProof struct {
	Message   *AggregateAttestationAndProof `json:"message"`
	Signature string                        `json:"signature"`
}

type AggregateAttestationAndProof struct {
	AggregatorIndex string       `json:"aggregator_index"`
	Aggregate       *Attestation `json:"aggregate"`
	SelectionProof  string       `json:"selection_proof"`
}

type SyncCommitteeSubscription struct {
	ValidatorIndex       string   `json:"validator_index"`
	SyncCommitteeIndices []string `json:"sync_committee_indices"`
	UntilEpoch           string   `json:"until_epoch"`
}

type BeaconCommitteeSubscription struct {
	ValidatorIndex   string `json:"validator_index"`
	CommitteeIndex   string `json:"committee_index"`
	CommitteesAtSlot string `json:"committees_at_slot"`
	Slot             string `json:"slot"`
	IsAggregator     bool   `json:"is_aggregator"`
}

type ValidatorRegistration struct {
	FeeRecipient string `json:"fee_recipient"`
	GasLimit     string `json:"gas_limit"`
	Timestamp    string `json:"timestamp"`
	Pubkey       string `json:"pubkey"`
}

type SignedValidatorRegistration struct {
	Message   *ValidatorRegistration `json:"message"`
	Signature string                 `json:"signature"`
}

type FeeRecipient struct {
	ValidatorIndex string `json:"validator_index"`
	FeeRecipient   string `json:"fee_recipient"`
}

type SignedVoluntaryExit struct {
	Message   *VoluntaryExit `json:"message"`
	Signature string         `json:"signature"`
}

type VoluntaryExit struct {
	Epoch          string `json:"epoch"`
	ValidatorIndex string `json:"validator_index"`
}

type Fork struct {
	PreviousVersion string `json:"previous_version"`
	CurrentVersion  string `json:"current_version"`
	Epoch           string `json:"epoch"`
}

func (s *Fork) ToConsensus() (*qrysmpb.Fork, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "Fork")
	}
	previousVersion, err := hexutil.Decode(s.PreviousVersion)
	if err != nil {
		return nil, NewDecodeError(err, "PreviousVersion")
	}
	currentVersion, err := hexutil.Decode(s.CurrentVersion)
	if err != nil {
		return nil, NewDecodeError(err, "CurrentVersion")
	}
	epoch, err := strconv.ParseUint(s.Epoch, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Epoch")
	}
	return &qrysmpb.Fork{
		PreviousVersion: previousVersion,
		CurrentVersion:  currentVersion,
		Epoch:           primitives.Epoch(epoch),
	}, nil
}

type SyncCommitteeMessage struct {
	Slot            string `json:"slot"`
	BeaconBlockRoot string `json:"beacon_block_root"`
	ValidatorIndex  string `json:"validator_index"`
	Signature       string `json:"signature"`
}

func (s *SignedValidatorRegistration) ToConsensus() (*qrysmpb.SignedValidatorRegistrationV1, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "SignedValidatorRegistration")
	}
	if s.Message == nil {
		return nil, NewDecodeError(errNilValue, "Message")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Message")
	}
	sig, err := hexutil.Decode(s.Signature)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}
	if len(sig) != fieldparams.MLDSA87SignatureLength {
		return nil, fmt.Errorf("Signature length was %d when expecting length %d", len(sig), fieldparams.MLDSA87SignatureLength)
	}
	return &qrysmpb.SignedValidatorRegistrationV1{
		Message:   msg,
		Signature: sig,
	}, nil
}

func (s *ValidatorRegistration) ToConsensus() (*qrysmpb.ValidatorRegistrationV1, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "ValidatorRegistration")
	}
	feeRecipient, err := hexutil.DecodeQ(s.FeeRecipient)
	if err != nil {
		return nil, NewDecodeError(err, "FeeRecipient")
	}
	if len(feeRecipient) != fieldparams.FeeRecipientLength {
		return nil, fmt.Errorf("feeRecipient length was %d when expecting length %d", len(feeRecipient), fieldparams.FeeRecipientLength)
	}
	pubKey, err := hexutil.Decode(s.Pubkey)
	if err != nil {
		return nil, NewDecodeError(err, "FeeRecipient")
	}
	if len(pubKey) != fieldparams.MLDSA87PubkeyLength {
		return nil, fmt.Errorf("FeeRecipient length was %d when expecting length %d", len(pubKey), fieldparams.MLDSA87PubkeyLength)
	}
	gasLimit, err := strconv.ParseUint(s.GasLimit, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "GasLimit")
	}
	timestamp, err := strconv.ParseUint(s.Timestamp, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Timestamp")
	}
	return &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: feeRecipient,
		GasLimit:     gasLimit,
		Timestamp:    timestamp,
		Pubkey:       pubKey,
	}, nil
}

func ValidatorRegistrationFromConsensus(vr *qrysmpb.ValidatorRegistrationV1) (*ValidatorRegistration, error) {
	if vr == nil {
		return nil, errors.New("ValidatorRegistrationV1 is empty")
	}
	return &ValidatorRegistration{
		FeeRecipient: hexutil.EncodeQ(vr.FeeRecipient),
		GasLimit:     strconv.FormatUint(vr.GasLimit, 10),
		Timestamp:    strconv.FormatUint(vr.Timestamp, 10),
		Pubkey:       hexutil.Encode(vr.Pubkey),
	}, nil
}

func SignedValidatorRegistrationFromConsensus(vr *qrysmpb.SignedValidatorRegistrationV1) (*SignedValidatorRegistration, error) {
	if vr == nil {
		return nil, errors.New("SignedValidatorRegistrationV1 is empty")
	}
	v, err := ValidatorRegistrationFromConsensus(vr.Message)
	if err != nil {
		return nil, err
	}
	return &SignedValidatorRegistration{
		Message:   v,
		Signature: hexutil.Encode(vr.Signature),
	}, nil
}

func (s *SignedContributionAndProof) ToConsensus() (*qrysmpb.SignedContributionAndProof, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "SignedContributionAndProof")
	}
	if s.Message == nil {
		return nil, NewDecodeError(errNilValue, "Message")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Message")
	}
	sig, err := hexutil.Decode(s.Signature)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}

	return &qrysmpb.SignedContributionAndProof{
		Message:   msg,
		Signature: sig,
	}, nil
}

func (c *ContributionAndProof) ToConsensus() (*qrysmpb.ContributionAndProof, error) {
	if c == nil {
		return nil, NewDecodeError(errNilValue, "ContributionAndProof")
	}
	if c.Contribution == nil {
		return nil, NewDecodeError(errNilValue, "Contribution")
	}
	contribution, err := c.Contribution.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Contribution")
	}
	aggregatorIndex, err := strconv.ParseUint(c.AggregatorIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "AggregatorIndex")
	}
	selectionProof, err := hexutil.Decode(c.SelectionProof)
	if err != nil {
		return nil, NewDecodeError(err, "SelectionProof")
	}

	return &qrysmpb.ContributionAndProof{
		AggregatorIndex: primitives.ValidatorIndex(aggregatorIndex),
		Contribution:    contribution,
		SelectionProof:  selectionProof,
	}, nil
}

func (s *SyncCommitteeContribution) ToConsensus() (*qrysmpb.SyncCommitteeContribution, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "SyncCommitteeContribution")
	}
	slot, err := strconv.ParseUint(s.Slot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Slot")
	}
	bbRoot, err := hexutil.Decode(s.BeaconBlockRoot)
	if err != nil {
		return nil, NewDecodeError(err, "BeaconBlockRoot")
	}
	subcommitteeIndex, err := strconv.ParseUint(s.SubcommitteeIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "SubcommitteeIndex")
	}
	aggBits, err := hexutil.Decode(s.AggregationBits)
	if err != nil {
		return nil, NewDecodeError(err, "AggregationBits")
	}

	sigs := make([][]byte, len(s.Signatures))
	for i, hexSig := range s.Signatures {
		sig, err := hexutil.Decode(hexSig)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Signatures[%d]", i))
		}
		sigs[i] = sig
	}

	return &qrysmpb.SyncCommitteeContribution{
		Slot:              primitives.Slot(slot),
		BlockRoot:         bbRoot,
		SubcommitteeIndex: subcommitteeIndex,
		AggregationBits:   aggBits,
		Signatures:        sigs,
	}, nil
}

func (s *SignedAggregateAttestationAndProof) ToConsensus() (*qrysmpb.SignedAggregateAttestationAndProof, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "SignedAggregateAttestationAndProof")
	}
	if s.Message == nil {
		return nil, NewDecodeError(errNilValue, "Message")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Message")
	}
	sig, err := hexutil.Decode(s.Signature)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}

	return &qrysmpb.SignedAggregateAttestationAndProof{
		Message:   msg,
		Signature: sig,
	}, nil
}

func (a *AggregateAttestationAndProof) ToConsensus() (*qrysmpb.AggregateAttestationAndProof, error) {
	if a == nil {
		return nil, NewDecodeError(errNilValue, "AggregateAttestationAndProof")
	}
	if a.Aggregate == nil {
		return nil, NewDecodeError(errNilValue, "Aggregate")
	}
	aggIndex, err := strconv.ParseUint(a.AggregatorIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "AggregatorIndex")
	}
	agg, err := a.Aggregate.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Aggregate")
	}
	proof, err := hexutil.Decode(a.SelectionProof)
	if err != nil {
		return nil, NewDecodeError(err, "SelectionProof")
	}
	return &qrysmpb.AggregateAttestationAndProof{
		AggregatorIndex: primitives.ValidatorIndex(aggIndex),
		Aggregate:       agg,
		SelectionProof:  proof,
	}, nil
}

func (a *Attestation) ToConsensus() (*qrysmpb.Attestation, error) {
	if a == nil {
		return nil, NewDecodeError(errNilValue, "Attestation")
	}
	if a.Data == nil {
		return nil, NewDecodeError(errNilValue, "Data")
	}
	aggBits, err := hexutil.Decode(a.AggregationBits)
	if err != nil {
		return nil, NewDecodeError(err, "AggregationBits")
	}
	data, err := a.Data.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Data")
	}

	sigs := make([][]byte, len(a.Signatures))
	for i, hexSig := range a.Signatures {
		sig, err := hexutil.Decode(hexSig)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Signatures[%d]", i))
		}
		sigs[i] = sig
	}

	return &qrysmpb.Attestation{
		AggregationBits: aggBits,
		Data:            data,
		Signatures:      sigs,
	}, nil
}

func AttestationFromConsensus(a *qrysmpb.Attestation) *Attestation {
	sigs := make([]string, len(a.Signatures))
	for i, sig := range a.Signatures {
		sigs[i] = hexutil.Encode(sig)
	}

	return &Attestation{
		AggregationBits: hexutil.Encode(a.AggregationBits),
		Data:            AttestationDataFromConsensus(a.Data),
		Signatures:      sigs,
	}
}

func (a *AttestationData) ToConsensus() (*qrysmpb.AttestationData, error) {
	if a == nil {
		return nil, NewDecodeError(errNilValue, "AttestationData")
	}
	if a.Source == nil {
		return nil, NewDecodeError(errNilValue, "Source")
	}
	if a.Target == nil {
		return nil, NewDecodeError(errNilValue, "Target")
	}
	slot, err := strconv.ParseUint(a.Slot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Slot")
	}
	committeeIndex, err := strconv.ParseUint(a.CommitteeIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "CommitteeIndex")
	}
	bbRoot, err := hexutil.Decode(a.BeaconBlockRoot)
	if err != nil {
		return nil, NewDecodeError(err, "BeaconBlockRoot")
	}
	source, err := a.Source.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Source")
	}
	target, err := a.Target.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Target")
	}

	return &qrysmpb.AttestationData{
		Slot:            primitives.Slot(slot),
		CommitteeIndex:  primitives.CommitteeIndex(committeeIndex),
		BeaconBlockRoot: bbRoot,
		Source:          source,
		Target:          target,
	}, nil
}

func AttestationDataFromConsensus(a *qrysmpb.AttestationData) *AttestationData {
	return &AttestationData{
		Slot:            strconv.FormatUint(uint64(a.Slot), 10),
		CommitteeIndex:  strconv.FormatUint(uint64(a.CommitteeIndex), 10),
		BeaconBlockRoot: hexutil.Encode(a.BeaconBlockRoot),
		Source:          CheckpointFromConsensus(a.Source),
		Target:          CheckpointFromConsensus(a.Target),
	}
}

func (c *Checkpoint) ToConsensus() (*qrysmpb.Checkpoint, error) {
	if c == nil {
		return nil, NewDecodeError(errNilValue, "Checkpoint")
	}
	epoch, err := strconv.ParseUint(c.Epoch, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Epoch")
	}
	root, err := hexutil.Decode(c.Root)
	if err != nil {
		return nil, NewDecodeError(err, "Root")
	}

	return &qrysmpb.Checkpoint{
		Epoch: primitives.Epoch(epoch),
		Root:  root,
	}, nil
}

func CheckpointFromConsensus(c *qrysmpb.Checkpoint) *Checkpoint {
	return &Checkpoint{
		Epoch: strconv.FormatUint(uint64(c.Epoch), 10),
		Root:  hexutil.Encode(c.Root),
	}
}

func (s *SyncCommitteeSubscription) ToConsensus() (*validator.SyncCommitteeSubscription, error) {
	if s == nil {
		return nil, NewDecodeError(errNilValue, "SyncCommitteeSubscription")
	}
	index, err := strconv.ParseUint(s.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "ValidatorIndex")
	}
	scIndices := make([]uint64, len(s.SyncCommitteeIndices))
	for i, ix := range s.SyncCommitteeIndices {
		scIndices[i], err = strconv.ParseUint(ix, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("SyncCommitteeIndices[%d]", i))
		}
	}
	epoch, err := strconv.ParseUint(s.UntilEpoch, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "UntilEpoch")
	}

	return &validator.SyncCommitteeSubscription{
		ValidatorIndex:       primitives.ValidatorIndex(index),
		SyncCommitteeIndices: scIndices,
		UntilEpoch:           primitives.Epoch(epoch),
	}, nil
}

func (b *BeaconCommitteeSubscription) ToConsensus() (*validator.BeaconCommitteeSubscription, error) {
	if b == nil {
		return nil, NewDecodeError(errNilValue, "BeaconCommitteeSubscription")
	}
	valIndex, err := strconv.ParseUint(b.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "ValidatorIndex")
	}
	committeeIndex, err := strconv.ParseUint(b.CommitteeIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "CommitteeIndex")
	}
	committeesAtSlot, err := strconv.ParseUint(b.CommitteesAtSlot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "CommitteesAtSlot")
	}
	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Slot")
	}

	return &validator.BeaconCommitteeSubscription{
		ValidatorIndex:   primitives.ValidatorIndex(valIndex),
		CommitteeIndex:   primitives.CommitteeIndex(committeeIndex),
		CommitteesAtSlot: committeesAtSlot,
		Slot:             primitives.Slot(slot),
		IsAggregator:     b.IsAggregator,
	}, nil
}

func (e *SignedVoluntaryExit) ToConsensus() (*qrysmpb.SignedVoluntaryExit, error) {
	if e == nil {
		return nil, NewDecodeError(errNilValue, "SignedVoluntaryExit")
	}
	if e.Message == nil {
		return nil, NewDecodeError(errNilValue, "Message")
	}
	sig, err := hexutil.Decode(e.Signature)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}
	exit, err := e.Message.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Message")
	}

	return &qrysmpb.SignedVoluntaryExit{
		Exit:      exit,
		Signature: sig,
	}, nil
}

func SignedVoluntaryExitFromConsensus(e *qrysmpb.SignedVoluntaryExit) *SignedVoluntaryExit {
	return &SignedVoluntaryExit{
		Message:   VoluntaryExitFromConsensus(e.Exit),
		Signature: hexutil.Encode(e.Signature),
	}
}

func (e *VoluntaryExit) ToConsensus() (*qrysmpb.VoluntaryExit, error) {
	if e == nil {
		return nil, NewDecodeError(errNilValue, "VoluntaryExit")
	}
	epoch, err := strconv.ParseUint(e.Epoch, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Epoch")
	}
	valIndex, err := strconv.ParseUint(e.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "ValidatorIndex")
	}

	return &qrysmpb.VoluntaryExit{
		Epoch:          primitives.Epoch(epoch),
		ValidatorIndex: primitives.ValidatorIndex(valIndex),
	}, nil
}

func (m *SyncCommitteeMessage) ToConsensus() (*qrysmpb.SyncCommitteeMessage, error) {
	if m == nil {
		return nil, NewDecodeError(errNilValue, "SyncCommitteeMessage")
	}
	slot, err := strconv.ParseUint(m.Slot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Slot")
	}
	root, err := DecodeHexWithLength(m.BeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "BeaconBlockRoot")
	}
	valIndex, err := strconv.ParseUint(m.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "ValidatorIndex")
	}
	sig, err := DecodeHexWithLength(m.Signature, fieldparams.MLDSA87SignatureLength)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}
	if _, err := ml_dsa_87.SignatureFromBytes(sig); err != nil {
		return nil, NewDecodeError(err, "Signature")
	}

	return &qrysmpb.SyncCommitteeMessage{
		Slot:           primitives.Slot(slot),
		BlockRoot:      root,
		ValidatorIndex: primitives.ValidatorIndex(valIndex),
		Signature:      sig,
	}, nil
}

func VoluntaryExitFromConsensus(e *qrysmpb.VoluntaryExit) *VoluntaryExit {
	return &VoluntaryExit{
		Epoch:          strconv.FormatUint(uint64(e.Epoch), 10),
		ValidatorIndex: strconv.FormatUint(uint64(e.ValidatorIndex), 10),
	}
}

// SyncDetails contains information about node sync status.
type SyncDetails struct {
	HeadSlot     string `json:"head_slot"`
	SyncDistance string `json:"sync_distance"`
	IsSyncing    bool   `json:"is_syncing"`
	IsOptimistic bool   `json:"is_optimistic"`
	ElOffline    bool   `json:"el_offline"`
}

// SyncDetailsContainer is a wrapper for Data.
type SyncDetailsContainer struct {
	Data *SyncDetails `json:"data"`
}
