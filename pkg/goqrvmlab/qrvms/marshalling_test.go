package qrvms

import (
	"encoding/json"
	"testing"

	"github.com/theQRL/go-qrl/common/uint512"
	"github.com/theQRL/go-qrl/qrl/tracers/logger"
)

// Test that marshalling is valid json
func TestMarshalling(t *testing.T) {
	log := new(logger.StructLog)
	for i := range 10 {
		el := uint512.NewInt(uint64(i))
		log.Stack = append(log.Stack, *el)
	}
	if out := CustomMarshal(log); !json.Valid(out) {
		t.Fatalf("invalid json: %v", string(out))
	}
}

func BenchmarkMarshalling(b *testing.B) {

	log := new(logger.StructLog)
	for i := range 10 {
		el := uint512.NewInt(uint64(i))
		log.Stack = append(log.Stack, *el)
	}
	var outp1 []byte
	b.Run("json", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			outp1, _ = json.Marshal(log)
		}
	})
	var outp2 []byte
	b.Run("fast", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			outp2 = FastMarshal(log)
		}
	})
	b.Log(string(outp1))
	b.Log(string(outp2))
}
