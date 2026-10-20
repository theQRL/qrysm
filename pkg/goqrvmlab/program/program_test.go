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

package program

import (
	"math/big"
	"testing"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/pkg/goqrvmlab/ops"
)

func TestPush(t *testing.T) {
	address0, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		panic(err)
	}

	tests := []struct {
		input    any
		expected string
	}{
		// native ints
		{0, "6000"},
		{uint64(1), "6001"},
		{0xfff, "610fff"},
		// bigints
		{big.NewInt(0), "6000"},
		{big.NewInt(1), "6001"},
		{big.NewInt(0xfff), "610fff"},
		// Addresses
		{address0, "9f0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
		{&common.Address{}, "9f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"},
	}
	for i, tc := range tests {
		p := NewProgram()
		p.Push(tc.input)
		if got := p.Hex(); got != tc.expected {
			t.Errorf("test %d: got %v expected %v", i, got, tc.expected)
		}
	}
}
func TestCall(t *testing.T) {
	address1, err := common.NewAddressFromString("Q00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001337")
	if err != nil {
		panic(err)
	}
	{ // Nil gas
		p := NewProgram()
		p.Call(nil, address1, big.NewInt(1), 1, 2, 3, 4)
		exp := "600460036002600160019f000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000013375af1"
		if got := p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}
	{ // Non nil gas
		p := NewProgram()
		p.Call(big.NewInt(0xffff), address1, big.NewInt(1), 1, 2, 3, 4)
		exp := "600460036002600160019f0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000133761fffff1"
		if got := p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}
}

func TestMstore(t *testing.T) {

	{
		p := NewProgram()
		p.Mstore(common.FromHex("0xaabb"), 0)
		if exp, got := "60aa60005360bb600153", p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}

	{
		p := NewProgram()
		p.Mstore(common.FromHex("0xaabb"), 3)
		if exp, got := "60aa60035360bb600453", p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}

	{
		// 34 bytes
		data := common.FromHex("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
			"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
			"FFFF")

		p := NewProgram()
		p.Mstore(data, 0)
		exp := "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff60005260ff60205360ff602153"
		if got := p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}

}

func TestMemToStorage(t *testing.T) {
	{
		p := NewProgram()
		p.MemToStorage(0, 33, 1)
		if exp, got := "600051600155602051600255", p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}
}

func TestSstore(t *testing.T) {
	p := NewProgram()
	p.Sstore(0x1337, []byte("1234"))
	if exp, got := "633132333461133755", p.Hex(); got != exp {
		t.Errorf("got %v expected %v", got, exp)
	}
}

func TestReturnData(t *testing.T) {
	{
		p := NewProgram()
		p.ReturnData([]byte{0xFF})
		if exp, got := "60ff60005360016000f3", p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}
	{
		p := NewProgram()
		// 32 bytes
		data := common.FromHex("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
			"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
		p.ReturnData(data)
		if exp, got := "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff60005260206000f3", p.Hex(); got != exp {
			t.Errorf("got %v expected %v", got, exp)
		}
	}
}

func TestCreateAndCall(t *testing.T) {

	// A constructor that stores a slot
	ctor := NewProgram()
	ctor.Sstore(0, big.NewInt(5))

	// A runtime bytecode which reads the slot and returns
	deployed := NewProgram()
	deployed.Push(0)
	deployed.Op(ops.SLOAD) // [value] in stack
	deployed.Push(0)       // [value, 0]
	deployed.Op(ops.MSTORE)
	deployed.Return(0, 32)

	// Pack them
	ctor.ReturnData(deployed.Bytecode())
	// Verify constructor + runtime code
	{
		exp := "6005600055606060005360006001536054600253606060035360006004536052600553606060065360206007536060600853600060095360f3600a53600b6000f3"
		if got := ctor.Hex(); got != exp {
			t.Fatalf("1: got %v expected %v", got, exp)
		}
	}

	{ // Verify CREATE + CALL
		p := NewProgram()
		p.CreateAndCall(ctor.Bytecode(), false, ops.CALL)
		exp := "7f60056000556060600053600060015360546002536060600353600060045360526000527f600553606060065360206007536060600853600060095360f3600a53600b600060205260f3604053604160006000f060006000600060006000a55af15050"
		if got := p.Hex(); got != exp {
			t.Fatalf("2: got %v expected %v", got, exp)
		}
	}

	{ // Verify CREATE + DELEGATECALL
		p := NewProgram()
		p.CreateAndCall(ctor.Bytecode(), false, ops.DELEGATECALL)
		exp := "7f60056000556060600053600060015360546002536060600353600060045360526000527f600553606060065360206007536060600853600060095360f3600a53600b600060205260f3604053604160006000f06000600060006000a45af45050"
		if got := p.Hex(); got != exp {
			t.Fatalf("3: got %v expected %v", got, exp)
		}
	}

	{ // Verify CREATE2 + STATICCALL
		p := NewProgram()
		p.CreateAndCall(ctor.Bytecode(), true, ops.STATICCALL)
		exp := "7f60056000556060600053600060015360546002536060600353600060045360526000527f600553606060065360206007536060600853600060095360f3600a53600b600060205260f36040536000604160006000f56000600060006000a45afa5050"
		if got := p.Hex(); got != exp {
			t.Fatalf("2: got %v expected %v", got, exp)
		}
	}

}
