package apimiddleware

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSetInitialPublishBlockPostRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	endpoint := &apimiddleware.Endpoint{}
	s := struct {
		Message struct {
			Slot string
		} `json:"message"`
	}{}
	t.Run("Zond", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)

		slot := primitives.Slot(0)
		s.Message = struct{ Slot string }{Slot: strconv.FormatUint(uint64(slot), 10)}
		j, err := json.Marshal(s)
		require.NoError(t, err)
		var body bytes.Buffer
		_, err = body.Write(j)
		require.NoError(t, err)
		request := httptest.NewRequest("POST", "http://foo.example", &body)
		runDefault, errJson := setInitialPublishBlockPostRequest(endpoint, nil, request)
		require.Equal(t, true, errJson == nil)
		assert.Equal(t, apimiddleware.RunDefault(true), runDefault)
		assert.Equal(t, reflect.TypeFor[SignedBeaconBlockZondJson]().Name(), reflect.Indirect(reflect.ValueOf(endpoint.PostRequest)).Type().Name())
	})
}

func TestPreparePublishedBlock(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		endpoint := &apimiddleware.Endpoint{
			PostRequest: &SignedBeaconBlockZondJson{
				Message: &BeaconBlockZondJson{
					Body: &BeaconBlockBodyZondJson{},
				},
			},
		}
		errJson := preparePublishedBlock(endpoint, nil, nil)
		require.Equal(t, true, errJson == nil)
		_, ok := endpoint.PostRequest.(*zondPublishBlockRequestJson)
		assert.Equal(t, true, ok)
	})

	t.Run("unsupported block type", func(t *testing.T) {
		errJson := preparePublishedBlock(&apimiddleware.Endpoint{}, nil, nil)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported block type"))
	})
}

func TestSetInitialPublishBlindedBlockPostRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	endpoint := &apimiddleware.Endpoint{}
	s := struct {
		Message struct {
			Slot string
		} `json:"message"`
	}{}
	t.Run("Zond", func(t *testing.T) {
		slot := primitives.Slot(0)
		s.Message = struct{ Slot string }{Slot: strconv.FormatUint(uint64(slot), 10)}
		j, err := json.Marshal(s)
		require.NoError(t, err)
		var body bytes.Buffer
		_, err = body.Write(j)
		require.NoError(t, err)
		request := httptest.NewRequest("POST", "http://foo.example", &body)
		runDefault, errJson := setInitialPublishBlindedBlockPostRequest(endpoint, nil, request)
		require.Equal(t, true, errJson == nil)
		assert.Equal(t, apimiddleware.RunDefault(true), runDefault)
		assert.Equal(t, reflect.TypeFor[SignedBlindedBeaconBlockZondJson]().Name(), reflect.Indirect(reflect.ValueOf(endpoint.PostRequest)).Type().Name())
	})
}

func TestPreparePublishedBlindedBlock(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		endpoint := &apimiddleware.Endpoint{
			PostRequest: &SignedBlindedBeaconBlockZondJson{
				Message: &BlindedBeaconBlockZondJson{
					Body: &BlindedBeaconBlockBodyZondJson{},
				},
			},
		}
		errJson := preparePublishedBlindedBlock(endpoint, nil, nil)
		require.Equal(t, true, errJson == nil)
		_, ok := endpoint.PostRequest.(*zondPublishBlindedBlockRequestJson)
		assert.Equal(t, true, ok)
	})
	t.Run("unsupported block type", func(t *testing.T) {
		errJson := preparePublishedBlock(&apimiddleware.Endpoint{}, nil, nil)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported block type"))
	})
}

func TestPrepareValidatorAggregates(t *testing.T) {
	body := &tempSyncCommitteesResponseJson{
		Data: &tempSyncCommitteeValidatorsJson{
			Validators: []string{"1", "2"},
			ValidatorAggregates: []*tempSyncSubcommitteeValidatorsJson{
				{
					Validators: []string{"3", "4"},
				},
				{
					Validators: []string{"5"},
				},
			},
		},
	}
	bodyJson, err := json.Marshal(body)
	require.NoError(t, err)

	container := &SyncCommitteesResponseJson{}
	runDefault, errJson := prepareValidatorAggregates(bodyJson, container)
	require.Equal(t, nil, errJson)
	require.Equal(t, apimiddleware.RunDefault(false), runDefault)
	assert.DeepEqual(t, []string{"1", "2"}, container.Data.Validators)
	require.DeepEqual(t, [][]string{{"3", "4"}, {"5"}}, container.Data.ValidatorAggregates)
}

func TestSerializeBlock(t *testing.T) {
	t.Run("incorrect response type", func(t *testing.T) {
		response := &types.Empty{}
		runDefault, j, errJson := serializeBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "container is not of the correct type"))
	})

	t.Run("unsupported block version", func(t *testing.T) {
		response := &BlockResponseJson{
			Version: "unsupported",
		}
		runDefault, j, errJson := serializeBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported block version"))
	})
}

func TestSerializeBlindedBlock(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		response := &BlindedBlockResponseJson{
			Version: qrlpb.Version_ZOND.String(),
			Data: &SignedBlindedBeaconBlockContainerJson{
				ZondBlock: &BlindedBeaconBlockZondJson{
					Slot:          "1",
					ProposerIndex: "1",
					ParentRoot:    "root",
					StateRoot:     "root",
					Body: &BlindedBeaconBlockBodyZondJson{
						ExecutionPayloadHeader: &ExecutionPayloadHeaderZondJson{
							ParentHash:       "parent_hash",
							FeeRecipient:     "fee_recipient",
							StateRoot:        "state_root",
							ReceiptsRoot:     "receipts_root",
							LogsBloom:        "logs_bloom",
							PrevRandao:       "prev_randao",
							BlockNumber:      "block_number",
							GasLimit:         "gas_limit",
							GasUsed:          "gas_used",
							TimeStamp:        "time_stamp",
							ExtraData:        "extra_data",
							BaseFeePerGas:    "base_fee_per_gas",
							BlockHash:        "block_hash",
							TransactionsRoot: "transactions_root",
							WithdrawalsRoot:  "withdrawals_root",
						},
					},
				},
				Signature: "sig",
			},
			ExecutionOptimistic: true,
		}
		runDefault, j, errJson := serializeBlindedBlock(response)
		require.Equal(t, nil, errJson)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.NotNil(t, j)
		resp := &zondBlindedBlockResponseJson{}
		require.NoError(t, json.Unmarshal(j, resp))
		require.NotNil(t, resp.Data)
		require.NotNil(t, resp.Data.Message)
		beaconBlock := resp.Data.Message
		assert.Equal(t, "1", beaconBlock.Slot)
		assert.Equal(t, "1", beaconBlock.ProposerIndex)
		assert.Equal(t, "root", beaconBlock.ParentRoot)
		assert.Equal(t, "root", beaconBlock.StateRoot)
		assert.NotNil(t, beaconBlock.Body)
		payloadHeader := beaconBlock.Body.ExecutionPayloadHeader
		assert.NotNil(t, payloadHeader)
		assert.Equal(t, "parent_hash", payloadHeader.ParentHash)
		assert.Equal(t, "fee_recipient", payloadHeader.FeeRecipient)
		assert.Equal(t, "state_root", payloadHeader.StateRoot)
		assert.Equal(t, "receipts_root", payloadHeader.ReceiptsRoot)
		assert.Equal(t, "logs_bloom", payloadHeader.LogsBloom)
		assert.Equal(t, "prev_randao", payloadHeader.PrevRandao)
		assert.Equal(t, "block_number", payloadHeader.BlockNumber)
		assert.Equal(t, "gas_limit", payloadHeader.GasLimit)
		assert.Equal(t, "gas_used", payloadHeader.GasUsed)
		assert.Equal(t, "time_stamp", payloadHeader.TimeStamp)
		assert.Equal(t, "extra_data", payloadHeader.ExtraData)
		assert.Equal(t, "base_fee_per_gas", payloadHeader.BaseFeePerGas)
		assert.Equal(t, "block_hash", payloadHeader.BlockHash)
		assert.Equal(t, "transactions_root", payloadHeader.TransactionsRoot)
		assert.Equal(t, "withdrawals_root", payloadHeader.WithdrawalsRoot)
		assert.Equal(t, true, resp.ExecutionOptimistic)

	})

	t.Run("incorrect response type", func(t *testing.T) {
		response := &types.Empty{}
		runDefault, j, errJson := serializeBlindedBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "container is not of the correct type"))
	})

	t.Run("unsupported block version", func(t *testing.T) {
		response := &BlindedBlockResponseJson{
			Version: "unsupported",
		}
		runDefault, j, errJson := serializeBlindedBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported block version"))
	})
}

func TestSerializeState(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		response := &BeaconStateResponseJson{
			Version: qrlpb.Version_ZOND.String(),
			Data: &BeaconStateContainerJson{
				ZondState: &BeaconStateZondJson{},
			},
		}
		runDefault, j, errJson := serializeState(response)
		require.Equal(t, nil, errJson)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.NotNil(t, j)
		require.NoError(t, json.Unmarshal(j, &zondStateResponseJson{}))
	})

	t.Run("incorrect response type", func(t *testing.T) {
		runDefault, j, errJson := serializeState(&types.Empty{})
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "container is not of the correct type"))
	})

	t.Run("unsupported state version", func(t *testing.T) {
		response := &BeaconStateResponseJson{
			Version: "unsupported",
		}
		runDefault, j, errJson := serializeState(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported state version"))
	})
}

func TestSerializeProducedBlock(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		response := &ProduceBlockResponseJson{
			Version: qrlpb.Version_ZOND.String(),
			Data: &BeaconBlockContainerJson{
				ZondBlock: &BeaconBlockZondJson{
					Slot:          "1",
					ProposerIndex: "1",
					ParentRoot:    "root",
					StateRoot:     "root",
					Body:          &BeaconBlockBodyZondJson{},
				},
			},
		}
		runDefault, j, errJson := serializeProducedBlock(response)
		require.Equal(t, nil, errJson)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.NotNil(t, j)
		resp := &zondProduceBlockResponseJson{}
		require.NoError(t, json.Unmarshal(j, resp))
		require.NotNil(t, resp.Data)
		require.NotNil(t, resp.Data)
		beaconBlock := resp.Data
		assert.Equal(t, "1", beaconBlock.Slot)
		assert.Equal(t, "1", beaconBlock.ProposerIndex)
		assert.Equal(t, "root", beaconBlock.ParentRoot)
		assert.Equal(t, "root", beaconBlock.StateRoot)
		require.NotNil(t, beaconBlock.Body)
	})
	t.Run("incorrect response type", func(t *testing.T) {
		response := &types.Empty{}
		runDefault, j, errJson := serializeProducedBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "container is not of the correct type"))
	})

	t.Run("unsupported block version", func(t *testing.T) {
		response := &ProduceBlockResponseJson{
			Version: "unsupported",
		}
		runDefault, j, errJson := serializeProducedBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported block version"))
	})
}

func TestSerializeProduceBlindedBlock(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		response := &ProduceBlindedBlockResponseJson{
			Version: qrlpb.Version_ZOND.String(),
			Data: &BlindedBeaconBlockContainerJson{
				ZondBlock: &BlindedBeaconBlockZondJson{
					Slot:          "1",
					ProposerIndex: "1",
					ParentRoot:    "root",
					StateRoot:     "root",
					Body:          &BlindedBeaconBlockBodyZondJson{},
				},
			},
		}
		runDefault, j, errJson := serializeProducedBlindedBlock(response)
		require.Equal(t, nil, errJson)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.NotNil(t, j)
		resp := &zondProduceBlindedBlockResponseJson{}
		require.NoError(t, json.Unmarshal(j, resp))
		require.NotNil(t, resp.Data)
		beaconBlock := resp.Data
		assert.Equal(t, "1", beaconBlock.Slot)
		assert.Equal(t, "1", beaconBlock.ProposerIndex)
		assert.Equal(t, "root", beaconBlock.ParentRoot)
		assert.Equal(t, "root", beaconBlock.StateRoot)
		require.NotNil(t, beaconBlock.Body)
	})

	t.Run("incorrect response type", func(t *testing.T) {
		response := &types.Empty{}
		runDefault, j, errJson := serializeProducedBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "container is not of the correct type"))
	})

	t.Run("unsupported block version", func(t *testing.T) {
		response := &ProduceBlockResponseJson{
			Version: "unsupported",
		}
		runDefault, j, errJson := serializeProducedBlock(response)
		require.Equal(t, apimiddleware.RunDefault(false), runDefault)
		require.Equal(t, 0, len(j))
		require.NotNil(t, errJson)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "unsupported block version"))
	})
}

func TestPrepareForkChoiceResponse(t *testing.T) {
	dump := &ForkChoiceDumpJson{
		JustifiedCheckpoint: &CheckpointJson{
			Epoch: "justified",
			Root:  "justified",
		},
		FinalizedCheckpoint: &CheckpointJson{
			Epoch: "finalized",
			Root:  "finalized",
		},
		BestJustifiedCheckpoint: &CheckpointJson{
			Epoch: "best_justified",
			Root:  "best_justified",
		},
		UnrealizedJustifiedCheckpoint: &CheckpointJson{
			Epoch: "unrealized_justified",
			Root:  "unrealized_justified",
		},
		UnrealizedFinalizedCheckpoint: &CheckpointJson{
			Epoch: "unrealized_finalized",
			Root:  "unrealized_finalized",
		},
		ProposerBoostRoot:         "proposer_boost_root",
		PreviousProposerBoostRoot: "previous_proposer_boost_root",
		HeadRoot:                  "head_root",
		ForkChoiceNodes: []*ForkChoiceNodeJson{
			{
				Slot:                     "node1_slot",
				BlockRoot:                "node1_block_root",
				ParentRoot:               "node1_parent_root",
				JustifiedEpoch:           "node1_justified_epoch",
				FinalizedEpoch:           "node1_finalized_epoch",
				UnrealizedJustifiedEpoch: "node1_unrealized_justified_epoch",
				UnrealizedFinalizedEpoch: "node1_unrealized_finalized_epoch",
				Balance:                  "node1_balance",
				Weight:                   "node1_weight",
				ExecutionOptimistic:      false,
				ExecutionBlockHash:       "node1_execution_block_hash",
				TimeStamp:                "node1_time_stamp",
				Validity:                 "node1_validity",
			},
			{
				Slot:                     "node2_slot",
				BlockRoot:                "node2_block_root",
				ParentRoot:               "node2_parent_root",
				JustifiedEpoch:           "node2_justified_epoch",
				FinalizedEpoch:           "node2_finalized_epoch",
				UnrealizedJustifiedEpoch: "node2_unrealized_justified_epoch",
				UnrealizedFinalizedEpoch: "node2_unrealized_finalized_epoch",
				Balance:                  "node2_balance",
				Weight:                   "node2_weight",
				ExecutionOptimistic:      true,
				ExecutionBlockHash:       "node2_execution_block_hash",
				TimeStamp:                "node2_time_stamp",
				Validity:                 "node2_validity",
			},
		},
	}
	runDefault, j, errorJson := prepareForkChoiceResponse(dump)
	assert.Equal(t, nil, errorJson)
	assert.Equal(t, apimiddleware.RunDefault(false), runDefault)
	result := &ForkChoiceResponseJson{}
	require.NoError(t, json.Unmarshal(j, result))
	require.NotNil(t, result)
	assert.Equal(t, "justified", result.JustifiedCheckpoint.Epoch)
	assert.Equal(t, "justified", result.JustifiedCheckpoint.Root)
	assert.Equal(t, "finalized", result.FinalizedCheckpoint.Epoch)
	assert.Equal(t, "finalized", result.FinalizedCheckpoint.Root)
	assert.Equal(t, "best_justified", result.ExtraData.BestJustifiedCheckpoint.Epoch)
	assert.Equal(t, "best_justified", result.ExtraData.BestJustifiedCheckpoint.Root)
	assert.Equal(t, "unrealized_justified", result.ExtraData.UnrealizedJustifiedCheckpoint.Epoch)
	assert.Equal(t, "unrealized_justified", result.ExtraData.UnrealizedJustifiedCheckpoint.Root)
	assert.Equal(t, "unrealized_finalized", result.ExtraData.UnrealizedFinalizedCheckpoint.Epoch)
	assert.Equal(t, "unrealized_finalized", result.ExtraData.UnrealizedFinalizedCheckpoint.Root)
	assert.Equal(t, "proposer_boost_root", result.ExtraData.ProposerBoostRoot)
	assert.Equal(t, "previous_proposer_boost_root", result.ExtraData.PreviousProposerBoostRoot)
	assert.Equal(t, "head_root", result.ExtraData.HeadRoot)
	require.Equal(t, 2, len(result.ForkChoiceNodes))
	node1 := result.ForkChoiceNodes[0]
	require.NotNil(t, node1)
	assert.Equal(t, "node1_slot", node1.Slot)
	assert.Equal(t, "node1_block_root", node1.BlockRoot)
	assert.Equal(t, "node1_parent_root", node1.ParentRoot)
	assert.Equal(t, "node1_justified_epoch", node1.JustifiedEpoch)
	assert.Equal(t, "node1_finalized_epoch", node1.FinalizedEpoch)
	assert.Equal(t, "node1_unrealized_justified_epoch", node1.ExtraData.UnrealizedJustifiedEpoch)
	assert.Equal(t, "node1_unrealized_finalized_epoch", node1.ExtraData.UnrealizedFinalizedEpoch)
	assert.Equal(t, "node1_balance", node1.ExtraData.Balance)
	assert.Equal(t, "node1_weight", node1.Weight)
	assert.Equal(t, false, node1.ExtraData.ExecutionOptimistic)
	assert.Equal(t, "node1_execution_block_hash", node1.ExecutionBlockHash)
	assert.Equal(t, "node1_time_stamp", node1.ExtraData.TimeStamp)
	assert.Equal(t, "node1_validity", node1.Validity)
	node2 := result.ForkChoiceNodes[1]
	require.NotNil(t, node2)
	assert.Equal(t, "node2_slot", node2.Slot)
	assert.Equal(t, "node2_block_root", node2.BlockRoot)
	assert.Equal(t, "node2_parent_root", node2.ParentRoot)
	assert.Equal(t, "node2_justified_epoch", node2.JustifiedEpoch)
	assert.Equal(t, "node2_finalized_epoch", node2.FinalizedEpoch)
	assert.Equal(t, "node2_unrealized_justified_epoch", node2.ExtraData.UnrealizedJustifiedEpoch)
	assert.Equal(t, "node2_unrealized_finalized_epoch", node2.ExtraData.UnrealizedFinalizedEpoch)
	assert.Equal(t, "node2_balance", node2.ExtraData.Balance)
	assert.Equal(t, "node2_weight", node2.Weight)
	assert.Equal(t, true, node2.ExtraData.ExecutionOptimistic)
	assert.Equal(t, "node2_execution_block_hash", node2.ExecutionBlockHash)
	assert.Equal(t, "node2_time_stamp", node2.ExtraData.TimeStamp)
	assert.Equal(t, "node2_validity", node2.Validity)
}
