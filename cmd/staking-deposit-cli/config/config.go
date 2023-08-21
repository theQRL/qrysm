package config

import (
	"encoding/hex"
	"fmt"

	"github.com/cyyber/qrysm/v4/encoding/bytesutil"
)

const BETANET = "betanet"

type Config struct {
	ChainSettings map[string]*ChainSetting

	DomainDeposit [4]byte
}

type ChainSetting struct {
	Name                 string
	GenesisForkVersion   []byte
	GenesisValidatorRoot []byte
}

func ToHex(data string) []byte {
	output, err := hex.DecodeString(data)
	if err != nil {
		panic(fmt.Errorf("failed to decode string %s", data))
	}
	return output
}

func GetConfig() *Config {
	c := &Config{
		ChainSettings: map[string]*ChainSetting{
			BETANET: {
				Name:                 BETANET,
				GenesisForkVersion:   ToHex("20000089"),
				GenesisValidatorRoot: ToHex(""),
			},
		},
		DomainDeposit: bytesutil.Uint32ToBytes4(0x03000000),
	}
	return c
}
