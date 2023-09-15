package config

import (
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/misc"
)

const BETANET = "betanet"

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
			BETANET: {
				Name:                  BETANET,
				GenesisForkVersion:    ToHex("20000089"),
				GenesisValidatorsRoot: ToHex(""),
			},
		},
	}
	return c
}
