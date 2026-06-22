package config

import (
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
)

const (
	TESTNET = "testnet"
	BETANET = "betanet"
	MAINNET = "mainnet"
	DEV     = "dev"
)

type Config struct {
	ChainSettings map[string]*ChainSetting

	DomainDeposit [4]byte
}

type ChainSetting struct {
	Name                  string
	GenesisForkVersion    []byte
	GenesisValidatorsRoot []byte
}

func ToHex(data string) []byte {
	return misc.DecodeHex(data)
}

func GetConfig() *Config {
	c := &Config{
		ChainSettings: map[string]*ChainSetting{
			TESTNET: {
				Name:                  TESTNET,
				GenesisForkVersion:    ToHex("0x20000089"),
				GenesisValidatorsRoot: ToHex("0xfbcc79d4dcfd1063e8c8380397bf63e8e34d1ab37fe699f94b9ef1a18bde8781"),
			},
			BETANET: {
				Name:                  BETANET,
				GenesisForkVersion:    ToHex("0x20000089"),
				GenesisValidatorsRoot: ToHex("0x8e0aea32a97da3012c2c158bae29794fd08a098144dfee4ed016272035e0d6da"),
			},
			MAINNET: {
				Name:                  MAINNET,
				GenesisForkVersion:    ToHex("0x00000000"),
				GenesisValidatorsRoot: ToHex("0x8e0aea32a97da3012c2c158bae29794fd08a098144dfee4ed016272035e0d6da"),
			},
			DEV: {
				Name:                  DEV,
				GenesisForkVersion:    ToHex("0x10000038"),
				GenesisValidatorsRoot: ToHex("0x8e0aea32a97da3012c2c158bae29794fd08a098144dfee4ed016272035e0d6da"),
			},
		},
	}
	return c
}
