package signing_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func signingRootTestData(t *testing.T) (*qrysmpb.BeaconBlockHeader, []byte, []byte, []byte) {
	t.Helper()
	key, err := ml_dsa_87.SecretKeyFromSeed(make([]byte, fieldparams.MLDSA87SeedLength))
	require.NoError(t, err)
	header := &qrysmpb.BeaconBlockHeader{
		Slot:          42,
		ProposerIndex: 3,
		ParentRoot:    make([]byte, fieldparams.RootLength),
		StateRoot:     make([]byte, fieldparams.RootLength),
		BodyRoot:      make([]byte, fieldparams.RootLength),
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainBeaconProposer, []byte{1, 2, 3, 4}, nil)
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(header, domain)
	require.NoError(t, err)
	sig, err := key.SignDeterministic(root[:])
	require.NoError(t, err)
	return header, key.PublicKey().Marshal(), sig.Marshal(), domain
}

func TestSigningRoot_NilInputs(t *testing.T) {
	_, pubkey, signature, domain := signingRootTestData(t)
	var header *qrysmpb.BeaconBlockHeader
	var epoch *primitives.Epoch
	for _, tc := range []struct {
		name string
		call func() error
		want string
	}{
		{"compute_nil_interface", func() error {
			_, err := signing.ComputeSigningRoot(nil, domain)
			return err
		}, "nil signing object"},
		{"compute_typed_nil", func() error {
			_, err := signing.ComputeSigningRoot(header, domain)
			return err
		}, "nil signing object"},
		{"compute_typed_nil_scalar", func() error {
			_, err := signing.ComputeSigningRoot(epoch, domain)
			return err
		}, "nil signing object"},
		{"verify_nil_interface", func() error {
			return signing.VerifySigningRoot(nil, pubkey, signature, domain)
		}, "nil signing object"},
		{"verify_typed_nil", func() error {
			return signing.VerifySigningRoot(header, pubkey, signature, domain)
		}, "nil signing object"},
		{"verify_nil_header", func() error {
			return signing.VerifyBlockHeaderSigningRoot(nil, pubkey, signature, domain)
		}, "nil signing object"},
		{"signing_data_nil_callback", func() error {
			_, err := signing.SigningData(nil, domain)
			return err
		}, "nil signing root function"},
		{"verify_block_nil_callback", func() error {
			return signing.VerifyBlockSigningRoot(pubkey, signature, domain, nil)
		}, "nil signing root function"},
		{"block_batch_nil_callback", func() error {
			_, err := signing.BlockSignatureBatch(pubkey, signature, domain, nil)
			return err
		}, "nil signing root function"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, tc.want, tc.call())
		})
	}
}

func TestComputeSigningRoot_ValueObject(t *testing.T) {
	// HashRoot also has non-pointer implementations; checking for typed nils
	// must not panic on these values.
	_, err := signing.ComputeSigningRoot(primitives.Epoch(1), make([]byte, fieldparams.RootLength))
	require.NoError(t, err)
}

func TestSigningRoot_Verification(t *testing.T) {
	header, pubkey, signature, domain := signingRootTestData(t)
	changedSignature := bytes.Clone(signature)
	changedSignature[0] ^= 1
	changedPubkey := bytes.Clone(pubkey)
	changedPubkey[0] ^= 1
	zeroT1 := bytes.Clone(pubkey)
	clear(zeroT1[32:])
	changedDomain := bytes.Clone(domain)
	changedDomain[0] ^= 1
	changedHeader := qrysmpb.CopyBeaconBlockHeader(header)
	changedHeader.Slot++
	malformedHeader := qrysmpb.CopyBeaconBlockHeader(header)
	malformedHeader.ParentRoot = nil
	for _, tc := range []struct {
		name      string
		header    *qrysmpb.BeaconBlockHeader
		pubkey    []byte
		signature []byte
		domain    []byte
		valid     bool
	}{
		{"valid", header, pubkey, signature, domain, true},
		{"changed_message", changedHeader, pubkey, signature, domain, false},
		{"changed_domain", header, pubkey, signature, changedDomain, false},
		{"changed_public_key", header, changedPubkey, signature, domain, false},
		{"changed_signature", header, pubkey, changedSignature, domain, false},
		{"zero_signature", header, pubkey, make([]byte, len(signature)), domain, false},
		{"nil_signature", header, pubkey, nil, domain, false},
		{"legacy_signature_length", header, pubkey, make([]byte, 96), domain, false},
		{"short_signature", header, pubkey, signature[:len(signature)-1], domain, false},
		{"long_signature", header, pubkey, append(bytes.Clone(signature), 0), domain, false},
		{"nil_public_key", header, nil, signature, domain, false},
		{"short_public_key", header, pubkey[:len(pubkey)-1], signature, domain, false},
		{"long_public_key", header, append(bytes.Clone(pubkey), 0), signature, domain, false},
		{"zero_t1", header, zeroT1, signature, domain, false},
		{"nil_domain", header, pubkey, signature, nil, false},
		{"four_byte_domain", header, pubkey, signature, domain[:4], false},
		{"long_domain", header, pubkey, signature, append(bytes.Clone(domain), 0), false},
		{"malformed_object", malformedHeader, pubkey, signature, domain, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for name, verify := range map[string]func() error{
				"object": func() error {
					return signing.VerifySigningRoot(tc.header, tc.pubkey, tc.signature, tc.domain)
				},
				"header": func() error {
					return signing.VerifyBlockHeaderSigningRoot(tc.header, tc.pubkey, tc.signature, tc.domain)
				},
				"block": func() error {
					return signing.VerifyBlockSigningRoot(tc.pubkey, tc.signature, tc.domain, tc.header.HashTreeRoot)
				},
				"batch": func() error {
					batch, err := signing.BlockSignatureBatch(tc.pubkey, tc.signature, tc.domain, tc.header.HashTreeRoot)
					if err != nil {
						return err
					}
					valid, err := batch.Verify()
					if err != nil {
						return err
					}
					if !valid {
						return signing.ErrSigFailedToVerify
					}
					return nil
				},
			} {
				t.Run(name, func(t *testing.T) {
					err := verify()
					if tc.valid {
						require.NoError(t, err)
					} else if err == nil {
						t.Fatal("accepted invalid signing input")
					}
				})
			}
		})
	}
}

func TestSigningRoot_HashingError(t *testing.T) {
	_, pubkey, signature, domain := signingRootTestData(t)
	want := errors.New("hashing failed")
	rootFunc := func() ([32]byte, error) { return [32]byte{}, want }
	_, err := signing.SigningData(rootFunc, domain)
	require.ErrorIs(t, err, want)
	require.ErrorIs(t, signing.VerifyBlockSigningRoot(pubkey, signature, domain, rootFunc), want)
	_, err = signing.BlockSignatureBatch(pubkey, signature, domain, rootFunc)
	require.ErrorIs(t, err, want)
}

func TestComputeDomain_InvalidLengths(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version []byte
		root    []byte
		want    string
	}{
		{"empty_version", []byte{}, nil, "CurrentVersion"},
		{"short_version", []byte{1, 2}, nil, "CurrentVersion"},
		{"long_version", []byte{1, 2, 0, 0, 255}, nil, "CurrentVersion"},
		{"empty_root", nil, []byte{}, "GenesisValidatorsRoot"},
		{"short_root", nil, make([]byte, 31), "GenesisValidatorsRoot"},
		{"long_root", nil, make([]byte, 33), "GenesisValidatorsRoot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := signing.ComputeDomain(params.BeaconConfig().DomainBeaconProposer, tc.version, tc.root)
			require.ErrorContains(t, tc.want, err)
		})
	}
}

func TestComputeDomain_Defaults(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GenesisForkVersion = []byte{1, 2, 3, 4}
	params.OverrideBeaconConfig(cfg)
	got, err := signing.ComputeDomain(cfg.DomainBeaconProposer, nil, nil)
	require.NoError(t, err)
	want, err := signing.ComputeDomain(cfg.DomainBeaconProposer, cfg.GenesisForkVersion, cfg.ZeroHash[:])
	require.NoError(t, err)
	require.DeepEqual(t, want, got)

	cfg = cfg.Copy()
	cfg.GenesisForkVersion = []byte{1, 2}
	params.OverrideBeaconConfig(cfg)
	_, err = signing.ComputeDomain(cfg.DomainBeaconProposer, nil, nil)
	require.ErrorContains(t, "CurrentVersion", err)
}

func TestComputeForkDigest_RejectsCacheKeyAliases(t *testing.T) {
	version := []byte{0xdc, 0xba, 0x98, 0x76}
	root := bytes.Repeat([]byte{0x54}, fieldparams.RootLength)
	joined := append(bytes.Clone(version), root...)
	checkMalformed := func(t *testing.T) {
		t.Helper()
		for split := 0; split <= len(joined); split++ {
			if split == signing.ForkVersionByteLength {
				continue
			}
			_, err := signing.ComputeForkDigest(joined[:split], joined[split:])
			require.ErrorContains(t, "CurrentVersion", err, "split %d", split)
		}
	}
	checkMalformed(t)
	digest, err := signing.ComputeForkDigest(version, root)
	require.NoError(t, err)
	checkMalformed(t)
	cachedDigest, err := signing.ComputeForkDigest(version, root)
	require.NoError(t, err)
	require.Equal(t, digest, cachedDigest)
}
