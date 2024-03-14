package blocks

import (
	"context"
	"testing"

	fuzz "github.com/google/gofuzz"
	v "github.com/theQRL/qrysm/v4/beacon-chain/core/validators"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestFuzzProcessBlockHeader_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	block := &zondpb.SignedBeaconBlockCapella{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(block)

		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		if block.Block == nil || block.Block.Body == nil || block.Block.Body.Eth1Data == nil {
			continue
		}
		wsb, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		_, err = ProcessBlockHeader(context.Background(), s, wsb)
		_ = err
	}
}

func TestFuzzverifyDepositDataSigningRoot_10000(_ *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	var ba []byte
	var pubkey [field_params.DilithiumPubkeyLength]byte
	var sig [96]byte
	var domain [4]byte
	var p []byte
	var s []byte
	var d []byte
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(&ba)
		fuzzer.Fuzz(&pubkey)
		fuzzer.Fuzz(&sig)
		fuzzer.Fuzz(&domain)
		fuzzer.Fuzz(&p)
		fuzzer.Fuzz(&s)
		fuzzer.Fuzz(&d)
		err := verifySignature(ba, pubkey[:], sig[:], domain[:])
		_ = err
		err = verifySignature(ba, p, s, d)
		_ = err
	}
}

func TestFuzzProcessEth1DataInBlock_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	e := &zondpb.Eth1Data{}
	state, err := state_native.InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
	require.NoError(t, err)
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(e)
		s, err := ProcessEth1DataInBlock(context.Background(), state, e)
		if err != nil && s != nil {
			t.Fatalf("state should be nil on err. found: %v on error: %v for state: %v and eth1data: %v", s, err, state, e)
		}
	}
}

func TestFuzzareEth1DataEqual_10000(_ *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	eth1data := &zondpb.Eth1Data{}
	eth1data2 := &zondpb.Eth1Data{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(eth1data)
		fuzzer.Fuzz(eth1data2)
		AreEth1DataEqual(eth1data, eth1data2)
		AreEth1DataEqual(eth1data, eth1data)
	}
}

func TestFuzzEth1DataHasEnoughSupport_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	eth1data := &zondpb.Eth1Data{}
	var stateVotes []*zondpb.Eth1Data
	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(eth1data)
		fuzzer.Fuzz(&stateVotes)
		s, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Eth1DataVotes: stateVotes,
		})
		require.NoError(t, err)
		_, err = Eth1DataHasEnoughSupport(s, eth1data)
		_ = err
	}

}

func TestFuzzProcessBlockHeaderNoVerify_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	block := &zondpb.BeaconBlockCapella{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(block)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		_, err = ProcessBlockHeaderNoVerify(context.Background(), s, block.Slot, block.ProposerIndex, block.ParentRoot, []byte{})
		_ = err
	}
}

func TestFuzzProcessRandao_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	b := &zondpb.SignedBeaconBlockCapella{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(b)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		if b.Block == nil || b.Block.Body == nil {
			continue
		}
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := ProcessRandao(context.Background(), s, wsb)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, b)
		}
	}
}

func TestFuzzProcessRandaoNoVerify_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	blockBody := &zondpb.BeaconBlockBodyCapella{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(blockBody)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessRandaoNoVerify(s, blockBody.RandaoReveal)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, blockBody)
		}
	}
}

func TestFuzzProcessProposerSlashings_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	p := &zondpb.ProposerSlashing{}
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(p)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessProposerSlashings(ctx, s, []*zondpb.ProposerSlashing{p}, v.SlashValidator)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and slashing: %v", r, err, state, p)
		}
	}
}

func TestFuzzVerifyProposerSlashing_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	proposerSlashing := &zondpb.ProposerSlashing{}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(proposerSlashing)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		err = VerifyProposerSlashing(s, proposerSlashing)
		_ = err
	}
}

func TestFuzzProcessAttesterSlashings_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	a := &zondpb.AttesterSlashing{}
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(a)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessAttesterSlashings(ctx, s, []*zondpb.AttesterSlashing{a}, v.SlashValidator)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and slashing: %v", r, err, state, a)
		}
	}
}

func TestFuzzVerifyAttesterSlashing_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	attesterSlashing := &zondpb.AttesterSlashing{}
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(attesterSlashing)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		err = VerifyAttesterSlashing(ctx, s, attesterSlashing)
		_ = err
	}
}

func TestFuzzIsSlashableAttestationData_10000(_ *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	attestationData := &zondpb.AttestationData{}
	attestationData2 := &zondpb.AttestationData{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(attestationData)
		fuzzer.Fuzz(attestationData2)
		IsSlashableAttestationData(attestationData, attestationData2)
	}
}

func TestFuzzslashableAttesterIndices_10000(_ *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	attesterSlashing := &zondpb.AttesterSlashing{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(attesterSlashing)
		SlashableAttesterIndices(attesterSlashing)
	}
}

func TestFuzzVerifyIndexedAttestationn_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	idxAttestation := &zondpb.IndexedAttestation{}
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(idxAttestation)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		err = VerifyIndexedAttestation(ctx, s, idxAttestation)
		_ = err
	}
}

func TestFuzzVerifyAttestation_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	attestation := &zondpb.Attestation{}
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(attestation)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		err = VerifyAttestationSignatures(ctx, s, attestation)
		_ = err
	}
}

func TestFuzzProcessDeposits_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	deposits := make([]*zondpb.Deposit, 100)
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		for i := range deposits {
			fuzzer.Fuzz(deposits[i])
		}
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessDeposits(ctx, s, deposits)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposits)
		}
	}
}

func TestFuzzProcessPreGenesisDeposit_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	deposit := &zondpb.Deposit{}
	ctx := context.Background()

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(deposit)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessPreGenesisDeposits(ctx, s, []*zondpb.Deposit{deposit})
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposit)
		}
	}
}

func TestFuzzProcessDeposit_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	deposit := &zondpb.Deposit{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(deposit)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, _, err := ProcessDeposit(s, deposit, true)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposit)
		}
	}
}

func TestFuzzverifyDeposit_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	deposit := &zondpb.Deposit{}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(deposit)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		err = verifyDeposit(s, deposit)
		_ = err
	}
}

func TestFuzzProcessVoluntaryExits_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	e := &zondpb.SignedVoluntaryExit{}
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(e)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessVoluntaryExits(ctx, s, []*zondpb.SignedVoluntaryExit{e})
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and exit: %v", r, err, state, e)
		}
	}
}

func TestFuzzProcessVoluntaryExitsNoVerify_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	e := &zondpb.SignedVoluntaryExit{}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(e)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := ProcessVoluntaryExits(context.Background(), s, []*zondpb.SignedVoluntaryExit{e})
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, e)
		}
	}
}

func TestFuzzVerifyExit_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	ve := &zondpb.SignedVoluntaryExit{}
	rawVal := &zondpb.Validator{}
	fork := &zondpb.Fork{}
	var slot primitives.Slot

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(ve)
		fuzzer.Fuzz(rawVal)
		fuzzer.Fuzz(fork)
		fuzzer.Fuzz(&slot)

		state := &zondpb.BeaconStateCapella{
			Slot:                  slot,
			Fork:                  fork,
			GenesisValidatorsRoot: params.BeaconConfig().ZeroHash[:],
		}
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)

		val, err := state_native.NewValidator(&zondpb.Validator{})
		_ = err
		err = VerifyExitAndSignature(val, s, ve)
		_ = err
	}
}
