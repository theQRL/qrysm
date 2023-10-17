package shuffle

import "github.com/theQRL/qrysm/v4/consensus-types/primitives"

// ShuffleTestCase --
type ShuffleTestCase struct {
	Seed    string                      `yaml:"seed"`
	Count   uint64                      `yaml:"count"`
	Mapping []primitives.ValidatorIndex `yaml:"mapping"`
}
