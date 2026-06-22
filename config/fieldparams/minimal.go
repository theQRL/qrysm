//go:build minimal

package field_params

import (
	cryptomldsa87 "github.com/theQRL/go-qrllib/crypto/ml_dsa_87"
	walletcommon "github.com/theQRL/go-qrllib/wallet/common"
)

const (
	Preset                                = "minimal"
	BlockRootsLength                      = 64            // SLOTS_PER_HISTORICAL_ROOT
	StateRootsLength                      = 64            // SLOTS_PER_HISTORICAL_ROOT
	RandaoMixesLength                     = 64            // EPOCHS_PER_HISTORICAL_VECTOR
	HistoricalRootsLength                 = 16777216      // HISTORICAL_ROOTS_LIMIT
	ValidatorRegistryLimit                = 1099511627776 // VALIDATOR_REGISTRY_LIMIT
	ExecutionDataVotesLength              = 32            // SLOTS_PER_EXECUTION_VOTING_PERIOD
	PreviousEpochAttestationsLength       = 1024          // MAX_ATTESTATIONS * SLOTS_PER_EPOCH
	CurrentEpochAttestationsLength        = 1024          // MAX_ATTESTATIONS * SLOTS_PER_EPOCH
	SlashingsLength                       = 64            // EPOCHS_PER_SLASHINGS_VECTOR
	SyncCommitteeLength                   = 16            // SYNC_COMMITTEE_SIZE
	RootLength                            = 32            // RootLength defines the byte length of a Merkle root.
	ExtendedSeedLength                    = walletcommon.ExtendedSeedSize
	MLDSA87SeedLength                     = walletcommon.SeedSize                 // MLDSA87SeedLength defines the byte length of a ML-DSA-87 seed.
	MLDSA87SignatureLength                = cryptomldsa87.CRYPTO_BYTES            // MLDSA87SignatureLength defines the byte length of a ML-DSA-87 signature.
	MLDSA87PubkeyLength                   = cryptomldsa87.CRYPTO_PUBLIC_KEY_BYTES // MLDSA87PubkeyLength defines the byte length of a ML-DSA-87 public key.
	MaxTxsPerPayloadLength                = 1048576                               // MaxTxsPerPayloadLength defines the maximum number of transactions that can be included in a payload.
	MaxBytesPerTxLength                   = 1073741824                            // MaxBytesPerTxLength defines the maximum number of bytes that can be included in a transaction.
	FeeRecipientLength                    = walletcommon.AddressSize              // FeeRecipientLength defines the byte length of a fee recipient.
	WithdrawalCredentialsLength           = walletcommon.AddressSize              // WithdrawalCredentialsLength defines the byte length of withdrawal credentials.
	LogsBloomLength                       = 256                                   // LogsBloomLength defines the byte length of a logs bloom.
	VersionLength                         = 4                                     // VersionLength defines the byte length of a fork version number.
	SlotsPerEpoch                         = 8                                     // SlotsPerEpoch defines the number of slots per epoch.
	SyncCommitteeAggregationBytesLength   = 2                                     // SyncCommitteeAggregationBytesLength defines the sync committee aggregate bytes.
	SyncAggregateSyncCommitteeBytesLength = 2                                     // SyncAggregateSyncCommitteeBytesLength defines the length of sync committee bytes in a sync aggregate.
	MaxWithdrawalsPerPayload              = 4                                     // MaxWithdrawalsPerPayloadLength defines the maximum number of withdrawals that can be included in a payload.
)
