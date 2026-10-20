package field_params

// SSZ block-body list limits shared by the mainnet and minimal presets.
const (
	MaxProposerSlashings = 16
	MaxAttesterSlashings = 2
	MaxAttestations      = 4
	MaxDeposits          = 16
	MaxVoluntaryExits    = 16
)

// DepositProofLength is the fixed SSZ length of Deposit.Proof in both presets:
// the tree branch followed by the deposit-count mix-in.
const DepositProofLength = 33

// Both withdrawal limits are available for validating configuration files for
// either preset. MaxWithdrawalsPerPayload selects this binary's compiled limit.
const (
	MainnetMaxWithdrawalsPerPayload = 16
	MinimalMaxWithdrawalsPerPayload = 4
)
