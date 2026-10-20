package testing

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"testing"

	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/theQRL/go-bitfield"
	engine "github.com/theQRL/qrysm/proto/engine/v1"
	v1 "github.com/theQRL/qrysm/proto/qrl/v1"
	alpha "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type sszCodec interface {
	MarshalSSZ() ([]byte, error)
	UnmarshalSSZ([]byte) error
	HashTreeRoot() ([32]byte, error)
}

func TestSSZMarshal_RejectsUnrepresentableSize(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("constructing a 2^32-byte logical encoding requires 64-bit sizes")
	}
	type marshaler interface {
		sszCodec
		ssz.Marshaler
	}
	newPayload := func() *engine.ExecutionPayloadZond {
		return &engine.ExecutionPayloadZond{
			ParentHash: make([]byte, 32), FeeRecipient: make([]byte, 64),
			StateRoot: make([]byte, 32), ReceiptsRoot: make([]byte, 32),
			LogsBloom: make([]byte, 256), PrevRandao: make([]byte, 32),
			BaseFeePerGas: make([]byte, 32), BlockHash: make([]byte, 32),
			Transactions: [][]byte{{1, 2, 3}},
		}
	}
	alphaBlock := func(payload *engine.ExecutionPayloadZond) *alpha.BeaconBlockZond {
		return &alpha.BeaconBlockZond{
			ParentRoot: make([]byte, 32), StateRoot: make([]byte, 32),
			Body: &alpha.BeaconBlockBodyZond{
				RandaoReveal: make([]byte, 32), Graffiti: make([]byte, 32),
				ExecutionData:    &alpha.ExecutionData{DepositRoot: make([]byte, 32), BlockHash: make([]byte, 32)},
				SyncAggregate:    &alpha.SyncAggregate{SyncCommitteeBits: alpha.NewSyncCommitteeAggregationBits()},
				ExecutionPayload: payload,
			},
		}
	}
	v1Block := func(payload *engine.ExecutionPayloadZond) *v1.BeaconBlockZond {
		return &v1.BeaconBlockZond{
			ParentRoot: make([]byte, 32), StateRoot: make([]byte, 32),
			Body: &v1.BeaconBlockBodyZond{
				RandaoReveal: make([]byte, 32), Graffiti: make([]byte, 32),
				ExecutionData:    &v1.ExecutionData{DepositRoot: make([]byte, 32), BlockHash: make([]byte, 32)},
				SyncAggregate:    &v1.SyncAggregate{SyncCommitteeBits: alpha.NewSyncCommitteeAggregationBits()},
				ExecutionPayload: payload,
			},
		}
	}
	for name, wrap := range map[string]func(*engine.ExecutionPayloadZond) marshaler{
		"payload":     func(p *engine.ExecutionPayloadZond) marshaler { return p },
		"alpha block": func(p *engine.ExecutionPayloadZond) marshaler { return alphaBlock(p) },
		"alpha signed block": func(p *engine.ExecutionPayloadZond) marshaler {
			return &alpha.SignedBeaconBlockZond{Block: alphaBlock(p), Signature: make([]byte, 4627)}
		},
		"v1 block": func(p *engine.ExecutionPayloadZond) marshaler { return v1Block(p) },
		"v1 signed block": func(p *engine.ExecutionPayloadZond) marshaler {
			return &v1.SignedBeaconBlockZond{Message: v1Block(p), Signature: make([]byte, 4627)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			payload := newPayload()
			obj := wrap(payload)
			control, err := obj.MarshalSSZ()
			if err != nil {
				t.Fatalf("healthy control: %v", err)
			}
			root, err := obj.HashTreeRoot()
			if err != nil {
				t.Fatalf("healthy control hash: %v", err)
			}
			// Reusing one buffer keeps the test small while the logical encoding
			// reaches the SSZ offset boundary. Each transaction is within its limit.
			shared := make([]byte, 1<<20)
			payload.Transactions = make([][]byte, 4096)
			for i := range payload.Transactions {
				payload.Transactions[i] = shared
			}
			last := len(payload.Transactions) - 1
			for _, target := range []uint64{1 << 32, 1<<32 + 1} {
				payload.Transactions[last] = shared
				excess := uint64(obj.SizeSSZ()) - target
				payload.Transactions[last] = shared[:len(shared)-int(excess)]
				if uint64(obj.SizeSSZ()) != target {
					t.Fatal("incorrect boundary fixture")
				}
				if _, err := obj.MarshalSSZ(); !errors.Is(err, ssz.ErrSize) {
					t.Fatalf("size %d: expected size error, got %v", target, err)
				}
				backing := bytes.Repeat([]byte{0xab}, 64)
				prefix := backing[:3]
				out, err := obj.MarshalSSZTo(prefix)
				if !errors.Is(err, ssz.ErrSize) || !bytes.Equal(out, prefix) || !bytes.Equal(backing, bytes.Repeat([]byte{0xab}, 64)) {
					t.Fatalf("size %d: marshal-to must reject without changing destination: %v", target, err)
				}
			}
			payload.Transactions = [][]byte{{1, 2, 3}}
			encoded, err := obj.MarshalSSZ()
			if err != nil || !bytes.Equal(control, encoded) {
				t.Fatalf("healthy retry changed encoding: %v", err)
			}
			gotRoot, err := obj.HashTreeRoot()
			if err != nil || gotRoot != root {
				t.Fatalf("healthy retry changed root: %v", err)
			}
		})
	}
}

func TestSSZUnmarshal_InvalidTransactionOffset(t *testing.T) {
	// A payload with an empty extra-data field, one empty transaction, and no withdrawals.
	encoded := make([]byte, 560)
	binary.LittleEndian.PutUint32(encoded[480:484], 556)
	binary.LittleEndian.PutUint32(encoded[548:552], 556)
	binary.LittleEndian.PutUint32(encoded[552:556], 560)
	binary.LittleEndian.PutUint32(encoded[556:], 4)
	var payload engine.ExecutionPayloadZond
	if err := payload.UnmarshalSSZ(encoded); err != nil {
		t.Fatal(err)
	}
	malformed := bytes.Clone(encoded)
	// Claim thousands of offsets while providing only four transaction bytes.
	binary.LittleEndian.PutUint32(malformed[556:], 4096*4)
	var decodeErr error
	allocations := testing.AllocsPerRun(2, func() {
		decodeErr = payload.UnmarshalSSZ(malformed)
	})
	if decodeErr == nil {
		t.Fatal("invalid transaction offset accepted")
	}
	if allocations != 0 {
		t.Fatalf("invalid offset allocated before rejection: %v allocations", allocations)
	}
	if err := payload.UnmarshalSSZ(encoded); err != nil {
		t.Fatal(err)
	}
	if len(payload.Transactions) != 1 || len(payload.Transactions[0]) != 0 {
		t.Fatal("valid retry decoded incorrect transactions")
	}
}

func TestSSZBitvectorPadding(t *testing.T) {
	for _, value := range []byte{0, 0x0f, 0x10, 0x80, 0xff} {
		t.Run(fmt.Sprintf("%02x", value), func(t *testing.T) {
			obj := &alpha.MetaDataV1{Attnets: make([]byte, 8), Syncnets: bitfield.Bitvector4{value}}
			_, hashErr := obj.HashTreeRoot()
			_, marshalErr := obj.MarshalSSZ()
			var decoded alpha.MetaDataV1
			decodeErr := decoded.UnmarshalSSZ(append(make([]byte, 16), value))
			if value > 0x0f {
				if hashErr == nil || marshalErr == nil || decodeErr == nil {
					t.Fatalf("nonzero padding accepted: hash %v, marshal %v, decode %v", hashErr, marshalErr, decodeErr)
				}
			} else if hashErr != nil || marshalErr != nil || decodeErr != nil {
				t.Fatalf("valid bitvector rejected: hash %v, marshal %v, decode %v", hashErr, marshalErr, decodeErr)
			}
		})
	}
}

func alphaAttestation(bits bitfield.Bitlist) sszCodec {
	return &alpha.Attestation{
		AggregationBits: bits,
		Data: &alpha.AttestationData{
			BeaconBlockRoot: bytes.Repeat([]byte{1}, 32),
			Source:          &alpha.Checkpoint{Root: bytes.Repeat([]byte{2}, 32)},
			Target:          &alpha.Checkpoint{Root: bytes.Repeat([]byte{3}, 32)},
		},
	}
}

func v1Attestation(bits bitfield.Bitlist) sszCodec {
	return &v1.Attestation{
		AggregationBits: bits,
		Data: &v1.AttestationData{
			BeaconBlockRoot: bytes.Repeat([]byte{1}, 32),
			Source:          &v1.Checkpoint{Root: bytes.Repeat([]byte{2}, 32)},
			Target:          &v1.Checkpoint{Root: bytes.Repeat([]byte{3}, 32)},
		},
	}
}

func TestSSZUnmarshal_ReusedReceiver(t *testing.T) {
	factories := map[string]func(byte) sszCodec{
		"withdrawal": func(seed byte) sszCodec {
			return &engine.Withdrawal{Address: bytes.Repeat([]byte{seed}, 64), Amount: uint64(seed)}
		},
		"alpha attestation": func(seed byte) sszCodec {
			return alphaAttestation(bitfield.NewBitlist(uint64(seed)))
		},
		"v1 attestation": func(seed byte) sszCodec {
			return v1Attestation(bitfield.NewBitlist(uint64(seed)))
		},
		"execution payload": func(seed byte) sszCodec {
			return &engine.ExecutionPayloadZond{
				ParentHash: bytes.Repeat([]byte{seed}, 32), FeeRecipient: make([]byte, 64),
				StateRoot: make([]byte, 32), ReceiptsRoot: make([]byte, 32),
				LogsBloom: make([]byte, 256), PrevRandao: make([]byte, 32),
				ExtraData: bytes.Repeat([]byte{seed}, int(seed)), BaseFeePerGas: make([]byte, 32),
				BlockHash: make([]byte, 32), Transactions: [][]byte{{seed, seed}},
				Withdrawals: []*engine.Withdrawal{{Address: bytes.Repeat([]byte{seed}, 64)}},
			}
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			receiver := factory(1)
			for _, seed := range []byte{2, 3, 1} {
				input := factory(seed)
				encoded, err := input.MarshalSSZ()
				if err != nil {
					t.Fatal(err)
				}
				wantRoot, err := input.HashTreeRoot()
				if err != nil {
					t.Fatal(err)
				}
				if err := receiver.UnmarshalSSZ(encoded); err != nil {
					t.Fatal(err)
				}
				got, err := receiver.MarshalSSZ()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(encoded, got) {
					t.Fatal("reused receiver retained previous SSZ data")
				}
				gotRoot, err := receiver.HashTreeRoot()
				if err != nil {
					t.Fatal(err)
				}
				if gotRoot != wantRoot {
					t.Fatal("reused receiver has a different root")
				}
			}
		})
	}
}

func TestSSZBitlistBounds(t *testing.T) {
	for name, factory := range map[string]func(bitfield.Bitlist) sszCodec{"alpha": alphaAttestation, "v1": v1Attestation} {
		t.Run(name, func(t *testing.T) {
			cases := []struct {
				name  string
				bits  bitfield.Bitlist
				valid bool
			}{
				{"nil", nil, false}, {"missing delimiter", bitfield.Bitlist{0}, false},
				{"trailing zero", bitfield.Bitlist{1, 0}, false},
			}
			for _, size := range []uint64{0, 1, 31, 32, 33, 255, 256} {
				cases = append(cases, struct {
					name  string
					bits  bitfield.Bitlist
					valid bool
				}{fmt.Sprint(size), bitfield.NewBitlist(size), size <= 32})
			}
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					obj := factory(tt.bits)
					_, hashErr := obj.HashTreeRoot()
					encoded, marshalErr := obj.MarshalSSZ()
					if !tt.valid {
						if hashErr == nil || marshalErr == nil {
							t.Fatalf("invalid bitlist accepted: hash %v, marshal %v", hashErr, marshalErr)
						}
						return
					}
					if hashErr != nil || marshalErr != nil {
						t.Fatalf("valid bitlist rejected: hash %v, marshal %v", hashErr, marshalErr)
					}
					if err := factory(nil).UnmarshalSSZ(encoded); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}
