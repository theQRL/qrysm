//go:build !minimal

package field_params

import (
	cryptomldsa87 "github.com/theQRL/go-qrllib/crypto/ml_dsa_87"
	walletcommon "github.com/theQRL/go-qrllib/wallet/common"
)

const (
	Preset                                = "mainnet"
	BlockRootsLength                      = 1024          // SLOTS_PER_HISTORICAL_ROOT
	StateRootsLength                      = 1024          // SLOTS_PER_HISTORICAL_ROOT
	RandaoMixesLength                     = 65536         // EPOCHS_PER_HISTORICAL_VECTOR
	HistoricalRootsLength                 = 16777216      // HISTORICAL_ROOTS_LIMIT
	ValidatorRegistryLimit                = 1099511627776 // VALIDATOR_REGISTRY_LIMIT
	ExecutionDataVotesLength              = 512           // SLOTS_PER_EXECUTION_VOTING_PERIOD
	PreviousEpochAttestationsLength       = 512           // MAX_ATTESTATIONS * SLOTS_PER_EPOCH
	CurrentEpochAttestationsLength        = 512           // MAX_ATTESTATIONS * SLOTS_PER_EPOCH
	SlashingsLength                       = 512           // EPOCHS_PER_SLASHINGS_VECTOR
	SyncCommitteeLength                   = 128           // SYNC_COMMITTEE_SIZE  // TODO (cyyber) : Original value 512, new value needs to be decided
	MaxValidatorsPerCommittee             = 32            // Compiled SSZ limit for attestation bits, indices, and signatures.
	RootLength                            = 32            // RootLength defines the byte length of a Merkle root.
	RandaoRevealLength                    = 32            // RandaoRevealLength defines the byte length of a RANDAO hash-onion reveal.
	RandaoCommitmentLength                = 32            // RandaoCommitmentLength defines the byte length of a RANDAO hash-onion commitment.
	ExtendedSeedLength                    = walletcommon.ExtendedSeedSize
	MLDSA87SeedLength                     = walletcommon.SeedSize                 // MLDSA87SeedLength defines the byte length of a ML-DSA-87 seed.
	MLDSA87SignatureLength                = cryptomldsa87.CRYPTO_BYTES            // MLDSA87SignatureLength defines the byte length of a ML-DSA-87 signature.
	MLDSA87PubkeyLength                   = cryptomldsa87.CRYPTO_PUBLIC_KEY_BYTES // MLDSA87PubkeyLength defines the byte length of a ML-DSA-87 public key.
	MaxTxsPerPayloadLength                = 1048576                               // MaxTxsPerPayloadLength defines the maximum number of transactions that can be included in a payload.
	MaxBytesPerTxLength                   = 1073741824                            // MaxBytesPerTxLength defines the maximum number of bytes that can be included in a transaction.
	FeeRecipientLength                    = walletcommon.AddressSize              // FeeRecipientLength defines the byte length of a fee recipient.
	WithdrawalRecipientLength             = walletcommon.AddressSize              // WithdrawalRecipientLength defines the byte length of the withdrawal recipient.
	LogsBloomLength                       = 256                                   // LogsBloomLength defines the byte length of a logs bloom.
	VersionLength                         = 4                                     // VersionLength defines the byte length of a fork version number.
	SlotsPerEpoch                         = 128                                   // SlotsPerEpoch defines the number of slots per epoch.
	SyncCommitteeAggregationBytesLength   = 16                                    // SyncCommitteeAggregationBytesLength defines the length of sync committee aggregate bytes. // TODO (cyyber) : Original value 16, new value needs to be decided
	SyncAggregateSyncCommitteeBytesLength = 16                                    // SyncAggregateSyncCommitteeBytesLength defines the length of sync committee bytes in a sync aggregate. // TODO (cyyber) : Original value 64, new value needs to be decided
	MaxWithdrawalsPerPayload              = MainnetMaxWithdrawalsPerPayload       // Compiled SSZ withdrawal list limit.
)
