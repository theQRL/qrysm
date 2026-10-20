package equality_test

import (
	"testing"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/ssz/equality"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
)

func TestDeepEqualNamedScalars(t *testing.T) {
	type namedByte uint8
	type namedUint16 uint16
	type namedUint32 uint32
	type namedInt32 int32
	type namedBool bool
	for _, tc := range []struct {
		name string
		x, y any
		want bool
	}{
		{"SSZ uint64 equal", primitives.SSZUint64(^uint64(0)), primitives.SSZUint64(^uint64(0)), true},
		{"SSZ uint64 different", primitives.SSZUint64(1), primitives.SSZUint64(2), false},
		{"different named types", primitives.SSZUint64(1), primitives.Epoch(1), false},
		{"named byte equal", namedByte(255), namedByte(255), true},
		{"named byte different", namedByte(255), namedByte(0), false},
		{"named uint16 equal", namedUint16(65535), namedUint16(65535), true},
		{"named uint16 different", namedUint16(1), namedUint16(2), false},
		{"named uint32 equal", namedUint32(^uint32(0)), namedUint32(^uint32(0)), true},
		{"named uint32 different", namedUint32(1), namedUint32(2), false},
		{"named int32 equal", namedInt32(-1), namedInt32(-1), true},
		{"named int32 different", namedInt32(-1), namedInt32(1), false},
		{"named bool equal", namedBool(true), namedBool(true), true},
		{"named bool different", namedBool(true), namedBool(false), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, equality.DeepEqual(tc.x, tc.y))
		})
	}
	x, y := primitives.SSZUint64(7), primitives.SSZUint64(7)
	assert.Equal(t, true, equality.DeepEqual(&x, &y))
	y++
	assert.Equal(t, false, equality.DeepEqual(&x, &y))
}

func TestDeepEqualUnexportedScalars(t *testing.T) {
	type record struct {
		count   uint64
		active  bool
		balance int32
	}
	value := record{count: ^uint64(0), active: true, balance: -1}
	assert.Equal(t, true, equality.DeepEqual(value, value))
	assert.Equal(t, false, equality.DeepEqual(value, record{count: 0, active: true, balance: -1}))
	assert.Equal(t, false, equality.DeepEqual(value, record{count: ^uint64(0), active: false, balance: -1}))
	assert.Equal(t, false, equality.DeepEqual(value, record{count: ^uint64(0), active: true, balance: 1}))
}

func TestDeepEqualBasicTypes(t *testing.T) {
	assert.Equal(t, true, equality.DeepEqual(true, true))
	assert.Equal(t, false, equality.DeepEqual(true, false))

	assert.Equal(t, true, equality.DeepEqual(byte(222), byte(222)))
	assert.Equal(t, false, equality.DeepEqual(byte(222), byte(111)))

	assert.Equal(t, true, equality.DeepEqual(uint64(1234567890), uint64(1234567890)))
	assert.Equal(t, false, equality.DeepEqual(uint64(1234567890), uint64(987653210)))

	assert.Equal(t, true, equality.DeepEqual("hello", "hello"))
	assert.Equal(t, false, equality.DeepEqual("hello", "world"))

	assert.Equal(t, true, equality.DeepEqual([3]byte{1, 2, 3}, [3]byte{1, 2, 3}))
	assert.Equal(t, false, equality.DeepEqual([3]byte{1, 2, 3}, [3]byte{1, 2, 4}))

	var nilSlice1, nilSlice2 []byte
	assert.Equal(t, true, equality.DeepEqual(nilSlice1, nilSlice2))
	assert.Equal(t, true, equality.DeepEqual(nilSlice1, []byte{}))
	assert.Equal(t, true, equality.DeepEqual([]byte{1, 2, 3}, []byte{1, 2, 3}))
	assert.Equal(t, false, equality.DeepEqual([]byte{1, 2, 3}, []byte{1, 2, 4}))
}

func TestDeepEqualStructs(t *testing.T) {
	type Store struct {
		V1 uint64
		V2 []byte
	}
	store1 := Store{uint64(1234), nil}
	store2 := Store{uint64(1234), []byte{}}
	store3 := Store{uint64(4321), []byte{}}
	assert.Equal(t, true, equality.DeepEqual(store1, store2))
	assert.Equal(t, false, equality.DeepEqual(store1, store3))
}

func TestDeepEqualStructs_Unexported(t *testing.T) {
	type Store struct {
		V1           uint64
		V2           []byte
		dontIgnoreMe string
	}
	store1 := Store{uint64(1234), nil, "hi there"}
	store2 := Store{uint64(1234), []byte{}, "hi there"}
	store3 := Store{uint64(4321), []byte{}, "wow"}
	store4 := Store{uint64(4321), []byte{}, "bow wow"}
	assert.Equal(t, true, equality.DeepEqual(store1, store2))
	assert.Equal(t, false, equality.DeepEqual(store1, store3))
	assert.Equal(t, false, equality.DeepEqual(store3, store4))
}

func TestDeepEqualProto(t *testing.T) {
	var fork1, fork2 *qrysmpb.Fork
	assert.Equal(t, true, equality.DeepEqual(fork1, fork2))

	fork1 = &qrysmpb.Fork{
		PreviousVersion: []byte{123},
		CurrentVersion:  []byte{124},
		Epoch:           1234567890,
	}
	fork2 = &qrysmpb.Fork{
		PreviousVersion: []byte{123},
		CurrentVersion:  []byte{125},
		Epoch:           1234567890,
	}
	assert.Equal(t, true, equality.DeepEqual(fork1, fork1))
	assert.Equal(t, false, equality.DeepEqual(fork1, fork2))

	checkpoint1 := &qrysmpb.Checkpoint{
		Epoch: 1234567890,
		Root:  []byte{},
	}
	checkpoint2 := &qrysmpb.Checkpoint{
		Epoch: 1234567890,
		Root:  nil,
	}
	assert.Equal(t, true, equality.DeepEqual(checkpoint1, checkpoint2))
}

func Test_IsProto(t *testing.T) {
	tests := []struct {
		name string
		item any
		want bool
	}{
		{
			name: "uint64",
			item: 0,
			want: false,
		},
		{
			name: "string",
			item: "foobar cheese",
			want: false,
		},
		{
			name: "uint64 array",
			item: []uint64{1, 2, 3, 4, 5, 6},
			want: false,
		},
		{
			name: "Attestation",
			item: &qrysmpb.Attestation{},
			want: true,
		},
		{
			name: "Array of attestations",
			item: []*qrysmpb.Attestation{},
			want: true,
		},
		{
			name: "Map of attestations",
			item: make(map[uint64]*qrysmpb.Attestation),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equality.IsProto(tt.item); got != tt.want {
				t.Errorf("isProtoSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}
