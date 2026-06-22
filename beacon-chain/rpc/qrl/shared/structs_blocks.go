package shared

type SignedBeaconBlockZond struct {
	Message   *BeaconBlockZond `json:"message" validate:"required"`
	Signature string           `json:"signature" validate:"required"`
}

type BeaconBlockZond struct {
	Slot          string               `json:"slot" validate:"required"`
	ProposerIndex string               `json:"proposer_index" validate:"required"`
	ParentRoot    string               `json:"parent_root" validate:"required"`
	StateRoot     string               `json:"state_root" validate:"required"`
	Body          *BeaconBlockBodyZond `json:"body" validate:"required"`
}

type BeaconBlockBodyZond struct {
	RandaoReveal      string                 `json:"randao_reveal" validate:"required"`
	ExecutionData     *ExecutionData         `json:"execution_data" validate:"required"`
	Graffiti          string                 `json:"graffiti" validate:"required"`
	ProposerSlashings []*ProposerSlashing    `json:"proposer_slashings" validate:"required,dive"`
	AttesterSlashings []*AttesterSlashing    `json:"attester_slashings" validate:"required,dive"`
	Attestations      []*Attestation         `json:"attestations" validate:"required,dive"`
	Deposits          []*Deposit             `json:"deposits" validate:"required,dive"`
	VoluntaryExits    []*SignedVoluntaryExit `json:"voluntary_exits" validate:"required,dive"`
	SyncAggregate     *SyncAggregate         `json:"sync_aggregate" validate:"required"`
	ExecutionPayload  *ExecutionPayloadZond  `json:"execution_payload" validate:"required"`
}

type SignedBlindedBeaconBlockZond struct {
	Message   *BlindedBeaconBlockZond `json:"message" validate:"required"`
	Signature string                  `json:"signature" validate:"required"`
}

type BlindedBeaconBlockZond struct {
	Slot          string                      `json:"slot" validate:"required"`
	ProposerIndex string                      `json:"proposer_index" validate:"required"`
	ParentRoot    string                      `json:"parent_root" validate:"required"`
	StateRoot     string                      `json:"state_root" validate:"required"`
	Body          *BlindedBeaconBlockBodyZond `json:"body" validate:"required"`
}

type BlindedBeaconBlockBodyZond struct {
	RandaoReveal           string                      `json:"randao_reveal" validate:"required"`
	ExecutionData          *ExecutionData              `json:"execution_data" validate:"required"`
	Graffiti               string                      `json:"graffiti" validate:"required"`
	ProposerSlashings      []*ProposerSlashing         `json:"proposer_slashings" validate:"required,dive"`
	AttesterSlashings      []*AttesterSlashing         `json:"attester_slashings" validate:"required,dive"`
	Attestations           []*Attestation              `json:"attestations" validate:"required,dive"`
	Deposits               []*Deposit                  `json:"deposits" validate:"required,dive"`
	VoluntaryExits         []*SignedVoluntaryExit      `json:"voluntary_exits" validate:"required,dive"`
	SyncAggregate          *SyncAggregate              `json:"sync_aggregate" validate:"required"`
	ExecutionPayloadHeader *ExecutionPayloadHeaderZond `json:"execution_payload_header" validate:"required"`
}

type ExecutionData struct {
	DepositRoot  string `json:"deposit_root" validate:"required"`
	DepositCount string `json:"deposit_count" validate:"required"`
	BlockHash    string `json:"block_hash" validate:"required"`
}

type ProposerSlashing struct {
	SignedHeader1 *SignedBeaconBlockHeader `json:"signed_header_1" validate:"required"`
	SignedHeader2 *SignedBeaconBlockHeader `json:"signed_header_2" validate:"required"`
}

type AttesterSlashing struct {
	Attestation1 *IndexedAttestation `json:"attestation_1" validate:"required"`
	Attestation2 *IndexedAttestation `json:"attestation_2" validate:"required"`
}

type Deposit struct {
	Proof []string     `json:"proof" validate:"required,dive,hexadecimal"`
	Data  *DepositData `json:"data" validate:"required"`
}

type DepositData struct {
	Pubkey                string `json:"pubkey" validate:"required"`
	WithdrawalCredentials string `json:"withdrawal_credentials" validate:"required"`
	Amount                string `json:"amount" validate:"required"`
	Signature             string `json:"signature" validate:"required"`
}

type SignedBeaconBlockHeaderContainer struct {
	Header    *SignedBeaconBlockHeader `json:"header"`
	Root      string                   `json:"root"`
	Canonical bool                     `json:"canonical"`
}

type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader `json:"message" validate:"required"`
	Signature string             `json:"signature" validate:"required"`
}

type BeaconBlockHeader struct {
	Slot          string `json:"slot" validate:"required"`
	ProposerIndex string `json:"proposer_index" validate:"required"`
	ParentRoot    string `json:"parent_root" validate:"required"`
	StateRoot     string `json:"state_root" validate:"required"`
	BodyRoot      string `json:"body_root" validate:"required"`
}

type IndexedAttestation struct {
	AttestingIndices []string         `json:"attesting_indices" validate:"required,dive"`
	Data             *AttestationData `json:"data" validate:"required"`
	Signatures       []string         `json:"signatures" validate:"required"`
}

type SyncAggregate struct {
	SyncCommitteeBits       string   `json:"sync_committee_bits" validate:"required"`
	SyncCommitteeSignatures []string `json:"sync_committee_signatures" validate:"required"`
}

type ExecutionPayload struct {
	ParentHash    string   `json:"parent_hash" validate:"required"`
	FeeRecipient  string   `json:"fee_recipient" validate:"required"`
	StateRoot     string   `json:"state_root" validate:"required"`
	ReceiptsRoot  string   `json:"receipts_root" validate:"required"`
	LogsBloom     string   `json:"logs_bloom" validate:"required"`
	PrevRandao    string   `json:"prev_randao" validate:"required"`
	BlockNumber   string   `json:"block_number" validate:"required"`
	GasLimit      string   `json:"gas_limit" validate:"required"`
	GasUsed       string   `json:"gas_used" validate:"required"`
	Timestamp     string   `json:"timestamp" validate:"required"`
	ExtraData     string   `json:"extra_data" validate:"required"`
	BaseFeePerGas string   `json:"base_fee_per_gas" validate:"required"`
	BlockHash     string   `json:"block_hash" validate:"required"`
	Transactions  []string `json:"transactions" validate:"required,dive,hexadecimal"`
}

type ExecutionPayloadHeader struct {
	ParentHash       string `json:"parent_hash" validate:"required"`
	FeeRecipient     string `json:"fee_recipient" validate:"required"`
	StateRoot        string `json:"state_root" validate:"required"`
	ReceiptsRoot     string `json:"receipts_root" validate:"required"`
	LogsBloom        string `json:"logs_bloom" validate:"required"`
	PrevRandao       string `json:"prev_randao" validate:"required"`
	BlockNumber      string `json:"block_number" validate:"required"`
	GasLimit         string `json:"gas_limit" validate:"required"`
	GasUsed          string `json:"gas_used" validate:"required"`
	Timestamp        string `json:"timestamp" validate:"required"`
	ExtraData        string `json:"extra_data" validate:"required"`
	BaseFeePerGas    string `json:"base_fee_per_gas" validate:"required"`
	BlockHash        string `json:"block_hash" validate:"required"`
	TransactionsRoot string `json:"transactions_root" validate:"required"`
}

type ExecutionPayloadZond struct {
	ParentHash    string        `json:"parent_hash" validate:"required"`
	FeeRecipient  string        `json:"fee_recipient" validate:"required"`
	StateRoot     string        `json:"state_root" validate:"required"`
	ReceiptsRoot  string        `json:"receipts_root" validate:"required"`
	LogsBloom     string        `json:"logs_bloom" validate:"required"`
	PrevRandao    string        `json:"prev_randao" validate:"required"`
	BlockNumber   string        `json:"block_number" validate:"required"`
	GasLimit      string        `json:"gas_limit" validate:"required"`
	GasUsed       string        `json:"gas_used" validate:"required"`
	Timestamp     string        `json:"timestamp" validate:"required"`
	ExtraData     string        `json:"extra_data" validate:"required"`
	BaseFeePerGas string        `json:"base_fee_per_gas" validate:"required"`
	BlockHash     string        `json:"block_hash" validate:"required"`
	Transactions  []string      `json:"transactions" validate:"required,dive"`
	Withdrawals   []*Withdrawal `json:"withdrawals" validate:"required,dive"`
}

type ExecutionPayloadHeaderZond struct {
	ParentHash       string `json:"parent_hash" validate:"required"`
	FeeRecipient     string `json:"fee_recipient" validate:"required"`
	StateRoot        string `json:"state_root" validate:"required"`
	ReceiptsRoot     string `json:"receipts_root" validate:"required"`
	LogsBloom        string `json:"logs_bloom" validate:"required"`
	PrevRandao       string `json:"prev_randao" validate:"required"`
	BlockNumber      string `json:"block_number" validate:"required"`
	GasLimit         string `json:"gas_limit" validate:"required"`
	GasUsed          string `json:"gas_used" validate:"required"`
	Timestamp        string `json:"timestamp" validate:"required"`
	ExtraData        string `json:"extra_data" validate:"required"`
	BaseFeePerGas    string `json:"base_fee_per_gas" validate:"required"`
	BlockHash        string `json:"block_hash" validate:"required"`
	TransactionsRoot string `json:"transactions_root" validate:"required"`
	WithdrawalsRoot  string `json:"withdrawals_root" validate:"required"`
}

type Withdrawal struct {
	WithdrawalIndex  string `json:"index" validate:"required"`
	ValidatorIndex   string `json:"validator_index" validate:"required"`
	ExecutionAddress string `json:"address" validate:"required"`
	Amount           string `json:"amount" validate:"required"`
}
