// Copyright 2019 Martin Holst Swende
// This file is part of the goevmlab library.
//
// The library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the goevmlab library. If not, see <http://www.gnu.org/licenses/>.

package ops

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core/rawdb"
	"github.com/theQRL/go-qrl/core/state"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/core/vm"
	"github.com/theQRL/go-qrl/params"
)

// TestSanity checks the npops and npushes against the
// go-ethereum codebase
func TestSanity(t *testing.T) {

	for i := range 256 {

		// We have the EOF opcodes defined, geth doesn't yet.
		switch i {
		case 0x5c, 0x5d, 0x5e, 0xb0, 0xb1, 0xb3, 0xb4:
			continue

		}
		// Lookup the name via opcode
		gethOp := vm.OpCode(byte(i))
		ourOp := OpCode(byte(i))
		{
			exp, got := gethOp.String(), ourOp.String()
			if exp != got {
				t.Errorf("op 0x%x, got %v expected %v", i, got, exp)
			}
		}
		// Lookup opcode via name
		if name := ourOp.String(); !strings.HasPrefix(name, "opcode") {
			our := byte(StringToOp(name))
			if byte(our) != byte(i) {
				t.Errorf("name %v, got 0x%x expected 0x%x", name, our, byte(i))
			}
			geth := byte(vm.StringToOp(name))
			if byte(geth) != byte(i) {
				t.Errorf("name %v, got 0x%x expected 0x%x", name, geth, byte(i))
			}
		}
	}
}

func TestForkOpcodes(t *testing.T) {
	testForkOpcodes(t, "Zond")
}

func testForkOpcodes(t *testing.T, fork string) {
	f := LookupFork(fork)
	if f == nil {
		t.Fatalf("fork missing %v", fork)
	}
	config, err := LookupChainConfig(fork)
	if err != nil {
		t.Fatal(err)
	}
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Consume all gas while preparing the stack. The interpreter checks stack
	// bounds before charging the tested opcode. Most opcodes then run out of
	// gas; zero-cost halting opcodes can complete.
	runOpcode := func(op OpCode, stackSize int) error {
		code := append(bytes.Repeat([]byte{byte(vm.PUSH0)}, stackSize), byte(op))
		caller := vm.AccountRef(common.Address{})
		contract := vm.NewContract(caller, caller, new(big.Int), uint64(stackSize)*vm.GasQuickStep)
		contract.Code = code
		qrvm := vm.NewQRVM(vm.BlockContext{}, vm.TxContext{}, statedb, config, vm.Config{})
		_, err := qrvm.Interpreter().Run(contract, nil, false)
		return err
	}
	checkAccepted := func(t *testing.T, op OpCode, stackSize int) {
		t.Helper()
		err := runOpcode(op, stackSize)
		if err != nil && !errors.Is(err, vm.ErrOutOfGas) && !errors.Is(err, vm.ErrExecutionReverted) {
			t.Fatalf("opcode rejected with %d stack items: %v", stackSize, err)
		}
	}

	for i := range 256 {
		op := OpCode(i)
		t.Run(fmt.Sprintf("%02x_%s", i, op), func(t *testing.T) {
			pops, pushes := len(op.Pops()), len(op.Pushes())
			if !slices.Contains(f.ValidOpcodes, op) || op == INVALID {
				var invalid *vm.ErrInvalidOpCode
				if err := runOpcode(op, pops); !errors.As(err, &invalid) {
					t.Fatalf("expected invalid opcode, got %v", err)
				}
				return
			}
			if name := vm.OpCode(op).String(); name != op.String() {
				t.Fatalf("opcode name %q differs from VM name %q", op, name)
			}

			checkAccepted(t, op, pops)
			if pops > 0 {
				var underflow *vm.ErrStackUnderflow
				if err := runOpcode(op, pops-1); !errors.As(err, &underflow) {
					t.Fatalf("expected underflow with %d stack items, got %v", pops-1, err)
				}
			}

			maxStack := min(int(params.StackLimit), int(params.StackLimit)+pops-pushes)
			checkAccepted(t, op, maxStack)
			if maxStack < int(params.StackLimit) {
				var overflow *vm.ErrStackOverflow
				if err := runOpcode(op, maxStack+1); !errors.As(err, &overflow) {
					t.Fatalf("expected overflow with %d stack items, got %v", maxStack+1, err)
				}
			}
		})
	}
}
