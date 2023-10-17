package config

import (
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/misc"
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
				GenesisForkVersion:    ToHex("0x20000089"),
				GenesisValidatorsRoot: ToHex("0xadbacd278c79e71f4dd0c8975a8398eca12748437e0dcbed8efed700bf509c71"),
			},
		},
	}
	return c
}
