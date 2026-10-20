// Copyright 2020 Marius van der Wijden
// This file is part of the fuzzy-vm library.
//
// The fuzzy-vm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The fuzzy-vm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the fuzzy-vm library. If not, see <http://www.gnu.org/licenses/>.

// Package generator provides means to generate state tests for QRL.
package generator

import (
	"math/big"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/pkg/FuzzyVM/filler"
	"github.com/theQRL/qrysm/pkg/goqrvmlab/fuzzing"
	"github.com/theQRL/qrysm/pkg/goqrvmlab/program"
)

var (
	fork              = "Zond"
	sender, _         = common.NewAddressFromString("Q00000000000000000000000000000000be6c1fd78f40b86a24dc2d7d633e2912d71e5d166f8be2c850d5727f0adcc170c7741b784295eae0c4f28291d0928dc7")
	sk                = hexutil.MustDecode("0x45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	recursionLevel    = 0
	maxRecursionLevel = 10
	minJumpDistance   = 10
)

// GenerateProgram creates a new qrvm program and returns
// a gstMaker based on it as well as its program code.
func GenerateProgram(f *filler.Filler) (*fuzzing.GstMaker, []byte) {
	var (
		env = Environment{
			p:         program.NewProgram(),
			f:         f,
			jumptable: NewJumptable(uint64(minJumpDistance)),
		}
		strategies = newAccStrats(basicStrategies)
	)

	// Run for counter rounds
	counter := f.Byte()
	for i := 0; i < int(counter); i++ {
		// Select one of the strategies
		rnd := f.Byte()
		strategy := selectStrat(rnd, strategies)
		// Execute the strategy
		strategy.Execute(env)
	}
	code := env.jumptable.InsertJumps(env.p.Bytecode())
	return createGstMaker(f, code), code
}

func createGstMaker(fill *filler.Filler, code []byte) *fuzzing.GstMaker {
	gst := fuzzing.NewGstMaker()
	gst.EnableFork(fork)
	// Add sender
	gst.AddAccount(sender, fuzzing.GenesisAccount{
		Nonce: 0,
		// Used to be 0xffffffff, increased to prevent sender to little money exceptions
		// see: https://gist.github.com/MariusVanDerWijden/008b91a61de4b0fb831b72c24600ef59
		Balance: big.NewInt(0x3fffffffffffffff),
		Storage: make(map[common.Hash]common.StorageValue64),
		Code:    []byte{},
	})
	// Add code
	dest, _ := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000ca1100f022")
	gst.AddAccount(dest, fuzzing.GenesisAccount{
		Code:    code,
		Balance: new(big.Int),
		Storage: make(map[common.Hash]common.StorageValue64),
	})
	// Add the transaction
	tx := &fuzzing.StTransaction{
		GasLimit:   []uint64{20000000},
		Nonce:      0,
		Value:      []string{randHex(fill, 4)},
		Data:       []string{randHex(fill, 100)},
		GasPrice:   big.NewInt(0x80),
		To:         dest.Hex(),
		PrivateKey: sk,
		Sender:     sender,
	}
	gst.SetTx(tx)
	return gst
}

func randHex(fill *filler.Filler, max int) string {
	return hexutil.Encode(fill.ByteSlice(max))
}
