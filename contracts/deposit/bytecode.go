package deposit

import (
	_ "embed"
	"strings"
)

//go:embed bytecode.bin
var depositContractCreationBytecode string

// DepositContractCreationCodeHex returns the compiled QRVM creation bytecode.
func DepositContractCreationCodeHex() string {
	return strings.TrimSpace(depositContractCreationBytecode)
}

// DepositContractRuntimeCodeHex returns the runtime bytecode that must be placed
// directly into genesis allocations after constructor storage is precomputed.
func DepositContractRuntimeCodeHex() string {
	code := strings.TrimPrefix(DepositContractCreationCodeHex(), "0x")
	if marker := strings.LastIndex(code, "f3fe"); marker >= 0 {
		return "0x" + code[marker+len("f3fe"):]
	}
	return "0x" + code
}
