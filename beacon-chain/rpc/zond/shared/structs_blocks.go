package shared

type SignedBeaconBlockCapella struct {
	Message   *BeaconBlockCapella `json:"message" validate:"required"`
	Signature string              `json:"signature" validate:"required"`
}

type BeaconBlockCapella struct {
	Slot          string                  `json:"slot" validate:"required"`
	ProposerIndex string                  `json:"proposer_index" validate:"required"`
	ParentRoot    string                  `json:"parent_root" validate:"required"`
	StateRoot     string                  `json:"state_root" validate:"required"`
	Body          *BeaconBlockBodyCapella `json:"body" validate:"required"`
}

type BeaconBlockBodyCapella struct {
	RandaoReveal                string                              `json:"randao_reveal" validate:"required"`
	Eth1Data                    *Eth1Data                           `json:"eth1_data" validate:"required"`
	Graffiti                    string                              `json:"graffiti" validate:"required"`
	ProposerSlashings           []*ProposerSlashing                 `json:"proposer_slashings" validate:"required,dive"`
	AttesterSlashings           []*AttesterSlashing                 `json:"attester_slashings" validate:"required,dive"`
	Attestations                []*Attestation                      `json:"attestations" validate:"required,dive"`
	Deposits                    []*Deposit                          `json:"deposits" validate:"required,dive"`
	VoluntaryExits              []*SignedVoluntaryExit              `json:"voluntary_exits" validate:"required,dive"`
	SyncAggregate               *SyncAggregate                      `json:"sync_aggregate" validate:"required"`
	ExecutionPayload            *ExecutionPayloadCapella            `json:"execution_payload" validate:"required"`
	DilithiumToExecutionChanges []*SignedDilithiumToExecutionChange `json:"dilithium_to_execution_changes" validate:"required,dive"`
}

type SignedBlindedBeaconBlockCapella struct {
	Message   *BlindedBeaconBlockCapella `json:"message" validate:"required"`
	Signature string                     `json:"signature" validate:"required"`
}

type BlindedBeaconBlockCapella struct {
	Slot          string                         `json:"slot" validate:"required"`
	ProposerIndex string                         `json:"proposer_index" validate:"required"`
	ParentRoot    string                         `json:"parent_root" validate:"required"`
	StateRoot     string                         `json:"state_root" validate:"required"`
	Body          *BlindedBeaconBlockBodyCapella `json:"body" validate:"required"`
}

type BlindedBeaconBlockBodyCapella struct {
	RandaoReveal                string                              `json:"randao_reveal" validate:"required"`
	Eth1Data                    *Eth1Data                           `json:"eth1_data" validate:"required"`
	Graffiti                    string                              `json:"graffiti" validate:"required"`
	ProposerSlashings           []*ProposerSlashing                 `json:"proposer_slashings" validate:"required,dive"`
	AttesterSlashings           []*AttesterSlashing                 `json:"attester_slashings" validate:"required,dive"`
	Attestations                []*Attestation                      `json:"attestations" validate:"required,dive"`
	Deposits                    []*Deposit                          `json:"deposits" validate:"required,dive"`
	VoluntaryExits              []*SignedVoluntaryExit              `json:"voluntary_exits" validate:"required,dive"`
	SyncAggregate               *SyncAggregate                      `json:"sync_aggregate" validate:"required"`
	ExecutionPayloadHeader      *ExecutionPayloadHeaderCapella      `json:"execution_payload_header" validate:"required"`
	DilithiumToExecutionChanges []*SignedDilithiumToExecutionChange `json:"dilithium_to_execution_changes" validate:"required,dive"`
}

type Eth1Data struct {
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

type ExecutionPayloadCapella struct {
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

type ExecutionPayloadHeaderCapella struct {
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

type SignedDilithiumToExecutionChange struct {
	Message   *DilithiumToExecutionChange `json:"message" validate:"required"`
	Signature string                      `json:"signature" validate:"required"`
}

type DilithiumToExecutionChange struct {
	ValidatorIndex      string `json:"validator_index" validate:"required"`
	FromDilithiumPubkey string `json:"from_dilithium_pubkey" validate:"required"`
	ToExecutionAddress  string `json:"to_execution_address" validate:"required"`
}
