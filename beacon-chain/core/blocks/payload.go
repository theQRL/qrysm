package blocks

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	consensus_types "github.com/theQRL/qrysm/v4/consensus-types"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/time/slots"
)

var (
	ErrInvalidPayloadBlockHash  = errors.New("invalid payload block hash")
	ErrInvalidPayloadTimeStamp  = errors.New("invalid payload timestamp")
	ErrInvalidPayloadPrevRandao = errors.New("invalid payload previous randao")
)

// IsExecutionBlock returns whether the block has a non-empty ExecutionPayload.
//
// Spec code:
// def is_execution_block(block: ReadOnlyBeaconBlock) -> bool:
//
//	return block.body.execution_payload != ExecutionPayload()
func IsExecutionBlock(body interfaces.ReadOnlyBeaconBlockBody) (bool, error) {
	if body == nil {
		return false, errors.New("nil block body")
	}
	payload, err := body.Execution()
	switch {
	case errors.Is(err, consensus_types.ErrUnsupportedField):
		return false, nil
	case err != nil:
		return false, err
	default:
	}
	isEmpty, err := blocks.IsEmptyExecutionData(payload)
	if err != nil {
		return false, err
	}
	return !isEmpty, nil
}

// IsExecutionEnabled returns true if the beacon chain can begin executing.
// Meaning the payload header is beacon state is non-empty or the payload in block body is non-empty.
//
// Spec code:
// def is_execution_enabled(state: BeaconState, body: ReadOnlyBeaconBlockBody) -> bool:
//
//	return is_merge_block(state, body) or is_merge_complete(state)
func IsExecutionEnabled(st state.BeaconState, body interfaces.ReadOnlyBeaconBlockBody) (bool, error) {
	if st == nil || body == nil {
		return false, errors.New("nil state or block body")
	}

	header, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return false, err
	}
	return IsExecutionEnabledUsingHeader(header, body)
}

// IsExecutionEnabledUsingHeader returns true if the execution is enabled using post processed payload header and block body.
// This is an optimized version of IsExecutionEnabled where beacon state is not required as an argument.
func IsExecutionEnabledUsingHeader(header interfaces.ExecutionData, body interfaces.ReadOnlyBeaconBlockBody) (bool, error) {
	isEmpty, err := blocks.IsEmptyExecutionData(header)
	if err != nil {
		return false, err
	}
	if !isEmpty {
		return true, nil
	}
	return IsExecutionBlock(body)
}

// ValidatePayload validates if payload is valid versus input beacon state.
// These validation steps apply to both pre merge and post merge.
//
// Spec code:
//
//	# Verify random
//	assert payload.random == get_randao_mix(state, get_current_epoch(state))
//	# Verify timestamp
//	assert payload.timestamp == compute_timestamp_at_slot(state, state.slot)
func ValidatePayload(st state.BeaconState, payload interfaces.ExecutionData) error {
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	if err != nil {
		return err
	}

	if !bytes.Equal(payload.PrevRandao(), random) {
		return ErrInvalidPayloadPrevRandao
	}
	t, err := slots.ToTime(st.GenesisTime(), st.Slot())
	if err != nil {
		return err
	}
	if payload.Timestamp() != uint64(t.Unix()) {
		return ErrInvalidPayloadTimeStamp
	}
	return nil
}

// ProcessPayload processes input execution payload using beacon state.
// ValidatePayloadWhenMergeCompletes validates if payload is valid versus input beacon state.
// These validation steps ONLY apply to post merge.
//
// Spec code:
// def process_execution_payload(state: BeaconState, payload: ExecutionPayload, execution_engine: ExecutionEngine) -> None:
//
//	# Verify consistency of the parent hash with respect to the previous execution payload header
//	if is_merge_complete(state):
//	    assert payload.parent_hash == state.latest_execution_payload_header.block_hash
//	# Verify random
//	assert payload.random == get_randao_mix(state, get_current_epoch(state))
//	# Verify timestamp
//	assert payload.timestamp == compute_timestamp_at_slot(state, state.slot)
//	# Verify the execution payload is valid
//	assert execution_engine.execute_payload(payload)
//	# Cache execution payload header
//	state.latest_execution_payload_header = ExecutionPayloadHeader(
//	    parent_hash=payload.parent_hash,
//	    FeeRecipient=payload.FeeRecipient,
//	    state_root=payload.state_root,
//	    receipt_root=payload.receipt_root,
//	    logs_bloom=payload.logs_bloom,
//	    random=payload.random,
//	    block_number=payload.block_number,
//	    gas_limit=payload.gas_limit,
//	    gas_used=payload.gas_used,
//	    timestamp=payload.timestamp,
//	    extra_data=payload.extra_data,
//	    base_fee_per_gas=payload.base_fee_per_gas,
//	    block_hash=payload.block_hash,
//	    transactions_root=hash_tree_root(payload.transactions),
//	)
func ProcessPayload(st state.BeaconState, payload interfaces.ExecutionData) (state.BeaconState, error) {
	var err error

	st, err = ProcessWithdrawals(st, payload)
	if err != nil {
		return nil, errors.Wrap(err, "could not process withdrawals")
	}
	if err := ValidatePayload(st, payload); err != nil {
		return nil, err
	}
	if err := st.SetLatestExecutionPayloadHeader(payload); err != nil {
		return nil, err
	}
	return st, nil
}

// ValidatePayloadHeader validates the payload header.
func ValidatePayloadHeader(st state.BeaconState, header interfaces.ExecutionData) error {
	// Validate header's random mix matches with state in current epoch
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	if err != nil {
		return err
	}
	if !bytes.Equal(header.PrevRandao(), random) {
		return ErrInvalidPayloadPrevRandao
	}

	// Validate header's timestamp matches with state in current slot.
	t, err := slots.ToTime(st.GenesisTime(), st.Slot())
	if err != nil {
		return err
	}
	if header.Timestamp() != uint64(t.Unix()) {
		return ErrInvalidPayloadTimeStamp
	}
	return nil
}

// ProcessPayloadHeader processes the payload header.
func ProcessPayloadHeader(st state.BeaconState, header interfaces.ExecutionData) (state.BeaconState, error) {
	var err error
	st, err = ProcessWithdrawals(st, header)
	if err != nil {
		return nil, errors.Wrap(err, "could not process withdrawals")
	}
	if err := ValidatePayloadHeader(st, header); err != nil {
		return nil, err
	}
	if err := st.SetLatestExecutionPayloadHeader(header); err != nil {
		return nil, err
	}
	return st, nil
}

// GetBlockPayloadHash returns the hash of the execution payload of the block
func GetBlockPayloadHash(blk interfaces.ReadOnlyBeaconBlock) ([32]byte, error) {
	var payloadHash [32]byte
	payload, err := blk.Body().Execution()
	if err != nil {
		return payloadHash, err
	}
	return bytesutil.ToBytes32(payload.BlockHash()), nil
}
