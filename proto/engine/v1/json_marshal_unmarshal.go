package enginev1

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	gzondtypes "github.com/theQRL/go-zond/core/types"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/runtime/version"
)

var errExecutionUnmarshal = errors.New("unable to unmarshal execution engine data")

// PayloadIDBytes defines a custom type for Payload IDs used by the engine API
// client with proper JSON Marshal and Unmarshal methods to hex.
type PayloadIDBytes [8]byte

// MarshalJSON --
func (b PayloadIDBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(hexutil.Bytes(b[:]))
}

// ExecutionBlock is the response kind received by the zond_getBlockByHash and
// zomd_getBlockByNumber endpoints via JSON-RPC.
type ExecutionBlock struct {
	Version int
	gzondtypes.Header
	Hash         common.Hash               `json:"hash"`
	Transactions []*gzondtypes.Transaction `json:"transactions"`
	Withdrawals  []*Withdrawal             `json:"withdrawals"`
}

func (e *ExecutionBlock) MarshalJSON() ([]byte, error) {
	decoded := make(map[string]interface{})
	encodedHeader, err := e.Header.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(encodedHeader, &decoded); err != nil {
		return nil, err
	}
	decoded["hash"] = e.Hash.String()
	decoded["transactions"] = e.Transactions
	decoded["withdrawals"] = e.Withdrawals

	return json.Marshal(decoded)
}

func (e *ExecutionBlock) UnmarshalJSON(enc []byte) error {
	type transactionsJson struct {
		Transactions []*gzondtypes.Transaction `json:"transactions"`
	}
	type withdrawalsJson struct {
		Withdrawals []*withdrawalJSON `json:"withdrawals"`
	}

	if err := e.Header.UnmarshalJSON(enc); err != nil {
		return err
	}
	decoded := make(map[string]interface{})
	if err := json.Unmarshal(enc, &decoded); err != nil {
		return err
	}
	blockHashStr, ok := decoded["hash"].(string)
	if !ok {
		return errors.New("expected `hash` field in JSON response")
	}
	decodedHash, err := hexutil.Decode(blockHashStr)
	if err != nil {
		return err
	}
	e.Hash = common.BytesToHash(decodedHash)

	rawWithdrawals, ok := decoded["withdrawals"]
	if !ok || rawWithdrawals == nil {
		return errors.New("expected `withdrawals` field in JSON response")
	}
	e.Version = version.Capella
	j := &withdrawalsJson{}
	if err := json.Unmarshal(enc, j); err != nil {
		return err
	}
	ws := make([]*Withdrawal, len(j.Withdrawals))
	for i, wj := range j.Withdrawals {
		ws[i], err = wj.ToWithdrawal()
		if err != nil {
			return err
		}
	}
	e.Withdrawals = ws

	rawTxList, ok := decoded["transactions"]
	if !ok || rawTxList == nil {
		// Exit early if there are no transactions stored in the json payload.
		return nil
	}
	txsList, ok := rawTxList.([]interface{})
	if !ok {
		return errors.Errorf("expected transaction list to be of a slice interface type.")
	}
	for _, tx := range txsList {
		// If the transaction is just a hex string, do not attempt to
		// unmarshal into a full transaction object.
		if txItem, ok := tx.(string); ok && strings.HasPrefix(txItem, "0x") {
			return nil
		}
	}
	// If the block contains a list of transactions, we JSON unmarshal
	// them into a list of gzond transaction objects.
	txJson := &transactionsJson{}
	if err := json.Unmarshal(enc, txJson); err != nil {
		return err
	}
	e.Transactions = txJson.Transactions
	return nil
}

// UnmarshalJSON --
func (b *PayloadIDBytes) UnmarshalJSON(enc []byte) error {
	var res [8]byte
	if err := hexutil.UnmarshalFixedJSON(reflect.TypeOf(b), enc, res[:]); err != nil {
		return err
	}
	*b = res
	return nil
}

type withdrawalJSON struct {
	Index     *hexutil.Uint64 `json:"index"`
	Validator *hexutil.Uint64 `json:"validatorIndex"`
	Address   *common.Address `json:"address"`
	Amount    *hexutil.Uint64 `json:"amount"`
}

func (j *withdrawalJSON) ToWithdrawal() (*Withdrawal, error) {
	w := &Withdrawal{}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	if err := w.UnmarshalJSON(b); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Withdrawal) MarshalJSON() ([]byte, error) {
	index := hexutil.Uint64(w.Index)
	validatorIndex := hexutil.Uint64(w.ValidatorIndex)
	gwei := hexutil.Uint64(w.Amount)
	address := common.BytesToAddress(w.Address)
	return json.Marshal(withdrawalJSON{
		Index:     &index,
		Validator: &validatorIndex,
		Address:   &address,
		Amount:    &gwei,
	})
}

func (w *Withdrawal) UnmarshalJSON(enc []byte) error {
	dec := withdrawalJSON{}
	if err := json.Unmarshal(enc, &dec); err != nil {
		return err
	}
	if dec.Index == nil {
		return errors.New("missing withdrawal index")
	}
	if dec.Validator == nil {
		return errors.New("missing validator index")
	}
	if dec.Amount == nil {
		return errors.New("missing withdrawal amount")
	}
	if dec.Address == nil {
		return errors.New("missing execution address")
	}
	*w = Withdrawal{}
	w.Index = uint64(*dec.Index)
	w.ValidatorIndex = primitives.ValidatorIndex(*dec.Validator)
	w.Amount = uint64(*dec.Amount)
	w.Address = dec.Address.Bytes()
	return nil
}

type GetPayloadV2ResponseJson struct {
	ExecutionPayload *ExecutionPayloadCapellaJSON `json:"executionPayload"`
	BlockValue       string                       `json:"blockValue"`
}

type ExecutionPayloadCapellaJSON struct {
	ParentHash    *common.Hash    `json:"parentHash"`
	FeeRecipient  *common.Address `json:"feeRecipient"`
	StateRoot     *common.Hash    `json:"stateRoot"`
	ReceiptsRoot  *common.Hash    `json:"receiptsRoot"`
	LogsBloom     *hexutil.Bytes  `json:"logsBloom"`
	PrevRandao    *common.Hash    `json:"prevRandao"`
	BlockNumber   *hexutil.Uint64 `json:"blockNumber"`
	GasLimit      *hexutil.Uint64 `json:"gasLimit"`
	GasUsed       *hexutil.Uint64 `json:"gasUsed"`
	Timestamp     *hexutil.Uint64 `json:"timestamp"`
	ExtraData     hexutil.Bytes   `json:"extraData"`
	BaseFeePerGas string          `json:"baseFeePerGas"`
	BlockHash     *common.Hash    `json:"blockHash"`
	Transactions  []hexutil.Bytes `json:"transactions"`
	Withdrawals   []*Withdrawal   `json:"withdrawals"`
}

// MarshalJSON --
func (e *ExecutionPayloadCapella) MarshalJSON() ([]byte, error) {
	transactions := make([]hexutil.Bytes, len(e.Transactions))
	for i, tx := range e.Transactions {
		transactions[i] = tx
	}
	baseFee := new(big.Int).SetBytes(bytesutil.ReverseByteOrder(e.BaseFeePerGas))
	baseFeeHex := hexutil.EncodeBig(baseFee)
	pHash := common.BytesToHash(e.ParentHash)
	sRoot := common.BytesToHash(e.StateRoot)
	recRoot := common.BytesToHash(e.ReceiptsRoot)
	prevRan := common.BytesToHash(e.PrevRandao)
	bHash := common.BytesToHash(e.BlockHash)
	blockNum := hexutil.Uint64(e.BlockNumber)
	gasLimit := hexutil.Uint64(e.GasLimit)
	gasUsed := hexutil.Uint64(e.GasUsed)
	timeStamp := hexutil.Uint64(e.Timestamp)
	recipient := common.BytesToAddress(e.FeeRecipient)
	logsBloom := hexutil.Bytes(e.LogsBloom)
	if e.Withdrawals == nil {
		e.Withdrawals = make([]*Withdrawal, 0)
	}
	return json.Marshal(ExecutionPayloadCapellaJSON{
		ParentHash:    &pHash,
		FeeRecipient:  &recipient,
		StateRoot:     &sRoot,
		ReceiptsRoot:  &recRoot,
		LogsBloom:     &logsBloom,
		PrevRandao:    &prevRan,
		BlockNumber:   &blockNum,
		GasLimit:      &gasLimit,
		GasUsed:       &gasUsed,
		Timestamp:     &timeStamp,
		ExtraData:     e.ExtraData,
		BaseFeePerGas: baseFeeHex,
		BlockHash:     &bHash,
		Transactions:  transactions,
		Withdrawals:   e.Withdrawals,
	})
}

// UnmarshalJSON --
func (e *ExecutionPayloadCapellaWithValue) UnmarshalJSON(enc []byte) error {
	dec := GetPayloadV2ResponseJson{}
	if err := json.Unmarshal(enc, &dec); err != nil {
		return err
	}
	if dec.ExecutionPayload == nil {
		return errors.New("missing required field 'executionPayload' for ExecutionPayloadWithValue")
	}

	if dec.ExecutionPayload.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for ExecutionPayload")
	}
	if dec.ExecutionPayload.FeeRecipient == nil {
		return errors.New("missing required field 'feeRecipient' for ExecutionPayload")
	}
	if dec.ExecutionPayload.StateRoot == nil {
		return errors.New("missing required field 'stateRoot' for ExecutionPayload")
	}
	if dec.ExecutionPayload.ReceiptsRoot == nil {
		return errors.New("missing required field 'receiptsRoot' for ExecutableDataV1")
	}
	if dec.ExecutionPayload.LogsBloom == nil {
		return errors.New("missing required field 'logsBloom' for ExecutionPayload")
	}
	if dec.ExecutionPayload.PrevRandao == nil {
		return errors.New("missing required field 'prevRandao' for ExecutionPayload")
	}
	if dec.ExecutionPayload.ExtraData == nil {
		return errors.New("missing required field 'extraData' for ExecutionPayload")
	}
	if dec.ExecutionPayload.BlockHash == nil {
		return errors.New("missing required field 'blockHash' for ExecutionPayload")
	}
	if dec.ExecutionPayload.Transactions == nil {
		return errors.New("missing required field 'transactions' for ExecutionPayload")
	}
	if dec.ExecutionPayload.BlockNumber == nil {
		return errors.New("missing required field 'blockNumber' for ExecutionPayload")
	}
	if dec.ExecutionPayload.Timestamp == nil {
		return errors.New("missing required field 'timestamp' for ExecutionPayload")
	}
	if dec.ExecutionPayload.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for ExecutionPayload")
	}
	if dec.ExecutionPayload.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for ExecutionPayload")
	}

	*e = ExecutionPayloadCapellaWithValue{Payload: &ExecutionPayloadCapella{}}
	e.Payload.ParentHash = dec.ExecutionPayload.ParentHash.Bytes()
	e.Payload.FeeRecipient = dec.ExecutionPayload.FeeRecipient.Bytes()
	e.Payload.StateRoot = dec.ExecutionPayload.StateRoot.Bytes()
	e.Payload.ReceiptsRoot = dec.ExecutionPayload.ReceiptsRoot.Bytes()
	e.Payload.LogsBloom = *dec.ExecutionPayload.LogsBloom
	e.Payload.PrevRandao = dec.ExecutionPayload.PrevRandao.Bytes()
	e.Payload.BlockNumber = uint64(*dec.ExecutionPayload.BlockNumber)
	e.Payload.GasLimit = uint64(*dec.ExecutionPayload.GasLimit)
	e.Payload.GasUsed = uint64(*dec.ExecutionPayload.GasUsed)
	e.Payload.Timestamp = uint64(*dec.ExecutionPayload.Timestamp)
	e.Payload.ExtraData = dec.ExecutionPayload.ExtraData
	baseFee, err := hexutil.DecodeBig(dec.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return err
	}
	e.Payload.BaseFeePerGas = bytesutil.PadTo(bytesutil.ReverseByteOrder(baseFee.Bytes()), fieldparams.RootLength)
	e.Payload.BlockHash = dec.ExecutionPayload.BlockHash.Bytes()
	transactions := make([][]byte, len(dec.ExecutionPayload.Transactions))
	for i, tx := range dec.ExecutionPayload.Transactions {
		transactions[i] = tx
	}
	e.Payload.Transactions = transactions
	if dec.ExecutionPayload.Withdrawals == nil {
		dec.ExecutionPayload.Withdrawals = make([]*Withdrawal, 0)
	}
	e.Payload.Withdrawals = dec.ExecutionPayload.Withdrawals

	v, err := hexutil.DecodeBig(dec.BlockValue)
	if err != nil {
		return err
	}
	e.Value = bytesutil.PadTo(bytesutil.ReverseByteOrder(v.Bytes()), fieldparams.RootLength)

	return nil
}

type payloadAttributesV2JSON struct {
	Timestamp             hexutil.Uint64 `json:"timestamp"`
	PrevRandao            hexutil.Bytes  `json:"prevRandao"`
	SuggestedFeeRecipient hexutil.BytesZ `json:"suggestedFeeRecipient"`
	Withdrawals           []*Withdrawal  `json:"withdrawals"`
}

// MarshalJSON --
func (p *PayloadAttributesV2) MarshalJSON() ([]byte, error) {
	withdrawals := p.Withdrawals
	if withdrawals == nil {
		withdrawals = make([]*Withdrawal, 0)
	}

	return json.Marshal(payloadAttributesV2JSON{
		Timestamp:             hexutil.Uint64(p.Timestamp),
		PrevRandao:            p.PrevRandao,
		SuggestedFeeRecipient: p.SuggestedFeeRecipient,
		Withdrawals:           withdrawals,
	})
}

func (p *PayloadAttributesV2) UnmarshalJSON(enc []byte) error {
	dec := payloadAttributesV2JSON{}
	if err := json.Unmarshal(enc, &dec); err != nil {
		return err
	}
	*p = PayloadAttributesV2{}
	p.Timestamp = uint64(dec.Timestamp)
	p.PrevRandao = dec.PrevRandao
	p.SuggestedFeeRecipient = dec.SuggestedFeeRecipient
	withdrawals := dec.Withdrawals
	if withdrawals == nil {
		withdrawals = make([]*Withdrawal, 0)
	}
	p.Withdrawals = withdrawals
	return nil
}

type payloadStatusJSON struct {
	LatestValidHash *common.Hash `json:"latestValidHash"`
	Status          string       `json:"status"`
	ValidationError *string      `json:"validationError"`
}

// MarshalJSON --
func (p *PayloadStatus) MarshalJSON() ([]byte, error) {
	var latestHash *common.Hash
	if p.LatestValidHash != nil {
		hash := common.Hash(bytesutil.ToBytes32(p.LatestValidHash))
		latestHash = &hash
	}
	return json.Marshal(payloadStatusJSON{
		LatestValidHash: latestHash,
		Status:          p.Status.String(),
		ValidationError: &p.ValidationError,
	})
}

// UnmarshalJSON --
func (p *PayloadStatus) UnmarshalJSON(enc []byte) error {
	dec := payloadStatusJSON{}
	if err := json.Unmarshal(enc, &dec); err != nil {
		return err
	}
	*p = PayloadStatus{}
	if dec.LatestValidHash != nil {
		p.LatestValidHash = dec.LatestValidHash[:]
	}
	p.Status = PayloadStatus_Status(PayloadStatus_Status_value[dec.Status])
	if dec.ValidationError != nil {
		p.ValidationError = *dec.ValidationError
	}
	return nil
}

type forkchoiceStateJSON struct {
	HeadBlockHash      hexutil.Bytes `json:"headBlockHash"`
	SafeBlockHash      hexutil.Bytes `json:"safeBlockHash"`
	FinalizedBlockHash hexutil.Bytes `json:"finalizedBlockHash"`
}

// MarshalJSON --
func (f *ForkchoiceState) MarshalJSON() ([]byte, error) {
	return json.Marshal(forkchoiceStateJSON{
		HeadBlockHash:      f.HeadBlockHash,
		SafeBlockHash:      f.SafeBlockHash,
		FinalizedBlockHash: f.FinalizedBlockHash,
	})
}

// UnmarshalJSON --
func (f *ForkchoiceState) UnmarshalJSON(enc []byte) error {
	dec := forkchoiceStateJSON{}
	if err := json.Unmarshal(enc, &dec); err != nil {
		return err
	}
	*f = ForkchoiceState{}
	f.HeadBlockHash = dec.HeadBlockHash
	f.SafeBlockHash = dec.SafeBlockHash
	f.FinalizedBlockHash = dec.FinalizedBlockHash
	return nil
}

type executionPayloadBodyV1JSON struct {
	Transactions []hexutil.Bytes `json:"transactions"`
	Withdrawals  []*Withdrawal   `json:"withdrawals"`
}

func (b *ExecutionPayloadBodyV1) MarshalJSON() ([]byte, error) {
	transactions := make([]hexutil.Bytes, len(b.Transactions))
	for i, tx := range b.Transactions {
		transactions[i] = tx
	}
	if len(b.Withdrawals) == 0 {
		b.Withdrawals = make([]*Withdrawal, 0)
	}
	return json.Marshal(executionPayloadBodyV1JSON{
		Transactions: transactions,
		Withdrawals:  b.Withdrawals,
	})
}

func (b *ExecutionPayloadBodyV1) UnmarshalJSON(enc []byte) error {
	var decoded *executionPayloadBodyV1JSON
	err := json.Unmarshal(enc, &decoded)
	if err != nil {
		return err
	}
	if len(decoded.Transactions) == 0 {
		b.Transactions = make([][]byte, 0)
	}
	if len(decoded.Withdrawals) == 0 {
		b.Withdrawals = make([]*Withdrawal, 0)
	}
	transactions := make([][]byte, len(decoded.Transactions))
	for i, tx := range decoded.Transactions {
		transactions[i] = tx
	}
	b.Transactions = transactions
	b.Withdrawals = decoded.Withdrawals
	return nil
}
