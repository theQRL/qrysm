package main

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/pkg/goqrvmlab/fuzzing"
	"github.com/theQRL/qrysm/pkg/goqrvmlab/ops"
	"github.com/theQRL/qrysm/pkg/goqrvmlab/program"
)

func main() {
	t := makeTest()
	out, _ := json.MarshalIndent(t, "", "  ")
	fmt.Println(string(out))
}

func makeTest() *fuzzing.GeneralStateTest {
	gst := fuzzing.BasicStateTest("Berlin")
	precompileAddress, _ := common.NewAddressFromString("Q00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000011")

	a, _ := common.NewAddressFromString("Q000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000aa")
	b, _ := common.NewAddressFromString("Q000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000bb")
	// 0xaa calls 0xbb, with exactly 0x2cef gas
	{
		aa := program.NewProgram()
		aa.Call(big.NewInt(0x2cef+21+36), b, 0, 0, 0, 0, 0)
		gst.AddAccount(a, fuzzing.GenesisAccount{
			Code:    aa.Bytecode(),
			Balance: big.NewInt(5),
			Storage: make(map[common.Hash]common.StorageValue64),
		})
	}
	// 0xbb does a call
	{
		bb := program.NewProgram()
		// expand memory
		// push the value
		bb.Push(1)
		// push the memory index
		bb.Push(0x100)
		bb.Op(ops.MSTORE)
		gas, ok := new(big.Int).SetString("7ef0367e633852132a0ebbf70eb714015dd44bc82e1e55a96ef1389c999c1bca", 16)
		if !ok {
			panic("nope")
		}
		// {"pc":21090,"op":241,"gas":"0x2cef","gasCost":"0x2cd3","memSize":0,"stack":["0x100","0x0","0x60","0x0","0x4b","0x11","0x7ef0367e633852132a0ebbf70eb714015dd44bc82e1e55a96ef1389c999c1bca"],
		bb.Call(gas, precompileAddress, 0x4b, 0x0, 0x60, 0x0, 0x100)
		bb.Op(ops.POP)
		bb.Return(0, 0)

		gst.AddAccount(b, fuzzing.GenesisAccount{
			Code:    bb.Bytecode(),
			Balance: big.NewInt(5),
			Storage: make(map[common.Hash]common.StorageValue64),
		})
	}

	// Create the precompile too
	//gst.AddAccount(precompileAddress, fuzzing.GenesisAccount{
	//	Balance: big.NewInt(5),
	//	Storage: make(map[common.Hash]common.StorageValue64),
	//})

	// The transaction from sender to a
	{
		fuzzing.AddTransaction(&a, gst)
	}

	return gst.ToGeneralStateTest("nethermind-caller")

}
