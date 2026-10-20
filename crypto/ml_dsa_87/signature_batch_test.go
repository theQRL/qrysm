package ml_dsa_87

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/ml_dsa_87t"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

const TestSignature = "test signature"

func TestCopySignatureSet(t *testing.T) {
	t.Run("ml_dsa_87t", func(t *testing.T) {
		key, err := RandKey()
		assert.NoError(t, err)
		key2, err := RandKey()
		assert.NoError(t, err)
		key3, err := RandKey()
		assert.NoError(t, err)

		message := [32]byte{'C', 'D'}
		message2 := [32]byte{'E', 'F'}
		message3 := [32]byte{'H', 'I'}

		sig, err := key.Sign(message[:])
		require.NoError(t, err)
		sig2, err := key2.Sign(message2[:])
		require.NoError(t, err)
		sig3, err := key3.Sign(message3[:])
		require.NoError(t, err)

		set := &SignatureBatch{
			Signatures:   [][][]byte{{sig.Marshal()}},
			PublicKeys:   [][]PublicKey{{key.PublicKey()}},
			Messages:     [][32]byte{message},
			Descriptions: createDescriptions(1),
		}
		set2 := &SignatureBatch{
			Signatures:   [][][]byte{{sig2.Marshal()}},
			PublicKeys:   [][]PublicKey{{key.PublicKey()}},
			Messages:     [][32]byte{message},
			Descriptions: createDescriptions(1),
		}
		set3 := &SignatureBatch{
			Signatures:   [][][]byte{{sig3.Marshal()}},
			PublicKeys:   [][]PublicKey{{key.PublicKey()}},
			Messages:     [][32]byte{message},
			Descriptions: createDescriptions(1),
		}
		aggSet := set.Join(set2).Join(set3)
		aggSet2 := aggSet.Copy()

		assert.DeepEqual(t, aggSet, aggSet2)
	})
}

func createDescriptions(length int, text ...string) []string {
	desc := make([]string, length)
	for i := range desc {
		if len(text) > 0 {
			desc[i] = text[0]
		} else {
			desc[i] = TestSignature
		}
	}
	return desc
}

func TestVerifyVerbosely_AllSignaturesValid(t *testing.T) {
	set := NewValidSignatureSet(t, "good", 3)
	valid, err := set.VerifyVerbosely()
	assert.NoError(t, err)
	assert.Equal(t, true, valid, "SignatureSet is expected to be valid")
}

func TestVerifyVerbosely_SomeSignaturesInvalid(t *testing.T) {
	goodSet := NewValidSignatureSet(t, "good", 3)
	badSet := NewInvalidSignatureSet(t, "bad", 3, false)
	set := NewSet().Join(goodSet).Join(badSet)
	valid, err := set.VerifyVerbosely()
	assert.Equal(t, false, valid, "SignatureSet is expected to be invalid")
	assert.StringContains(t, "signature 'signature of bad0' is invalid", err.Error())
	assert.StringContains(t, "signature 'signature of bad1' is invalid", err.Error())
	assert.StringContains(t, "signature 'signature of bad2' is invalid", err.Error())
	assert.StringNotContains(t, "signature 'signature of good0' is invalid", err.Error())
	assert.StringNotContains(t, "signature 'signature of good1' is invalid", err.Error())
	assert.StringNotContains(t, "signature 'signature of good2' is invalid", err.Error())
}

func TestVerifyVerbosely_VerificationThrowsError(t *testing.T) {
	goodSet := NewValidSignatureSet(t, "good", 1)
	badSet := NewInvalidSignatureSet(t, "bad", 1, true)
	set := NewSet().Join(goodSet).Join(badSet)
	valid, err := set.VerifyVerbosely()
	assert.Equal(t, false, valid, "SignatureSet is expected to be invalid")
	assert.StringContains(t, "signature 'signature of bad0' is invalid", err.Error())
	assert.StringNotContains(t, "signature 'signature of good0' is invalid", err.Error())
}

func TestVerifyVerbosely_RejectsNilPublicKey(t *testing.T) {
	set := NewValidSignatureSet(t, "nil key", 1)
	set.PublicKeys[0][0] = nil
	valid, err := set.VerifyVerbosely()
	require.ErrorContains(t, "invalid public key at batch 0, index 0", err)
	require.Equal(t, false, valid)
}

func TestVerifyVerbosely_BoundedDiagnostics(t *testing.T) {
	base := NewInvalidSignatureSet(t, "bad", 1, false)
	for _, tc := range []struct {
		name      string
		count     int
		longLabel bool
		oversized bool
	}{
		{name: "single", count: 1},
		{name: "sample_limit", count: 8},
		{name: "many_failures", count: 64},
		{name: "long_description", count: 64, longLabel: true},
		{name: "oversized_signature", count: 64, longLabel: true, oversized: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := base.Copy()
			if tc.longLabel {
				entry.Descriptions[0] = strings.Repeat("label", 1<<14)
			}
			if tc.oversized {
				entry.Signatures[0][0] = make([]byte, 1<<16)
			}
			set := NewSet()
			for range tc.count {
				set.Join(entry)
			}
			valid, err := set.VerifyVerbosely()
			require.Equal(t, false, valid)
			require.ErrorContains(t, "some signatures are invalid", err)
			diagnostic := err.Error()
			if len(diagnostic) > 4096 {
				t.Fatalf("diagnostics exceed 4 KiB: %d bytes", len(diagnostic))
			}
			reported := strings.Count(diagnostic, " is invalid.")
			if reported < 1 || reported > min(tc.count, 8) {
				t.Fatalf("unexpected number of diagnostic entries: %d", reported)
			}
			if reported < tc.count {
				require.ErrorContains(t, fmt.Sprintf("%d additional invalid signatures omitted", tc.count-reported), err)
			} else {
				assert.StringNotContains(t, "omitted", diagnostic)
			}
			assert.StringNotContains(t, fmt.Sprintf("%x", entry.Signatures[0][0]), diagnostic)
			assert.StringNotContains(t, fmt.Sprintf("%x", entry.PublicKeys[0][0].Marshal()), diagnostic)
			assert.StringContains(t, "signature prefix:", diagnostic)
			assert.StringContains(t, "public key prefix:", diagnostic)
			if tc.longLabel {
				assert.StringNotContains(t, entry.Descriptions[0], diagnostic)
				assert.StringContains(t, "...", diagnostic)
			}
			if tc.oversized {
				require.ErrorContains(t, "signature must be", err)
			}
		})
	}
}

func TestVerifyVerbosely_EmptyAndNil(t *testing.T) {
	valid, err := NewSet().VerifyVerbosely()
	require.NoError(t, err)
	require.Equal(t, false, valid)
	valid, err = (*SignatureBatch)(nil).VerifyVerbosely()
	require.ErrorContains(t, "nil signature set", err)
	require.Equal(t, false, valid)
}

func TestSignatureBatch_RejectsInvalidDescriptions(t *testing.T) {
	for _, num := range []int{1, 2} {
		base := NewValidSignatureSet(t, "description", num)
		for _, tc := range []struct {
			name         string
			descriptions []string
		}{
			{name: "nil"},
			{name: "empty", descriptions: []string{}},
			{name: "short", descriptions: createDescriptions(num - 1)},
			{name: "extra", descriptions: createDescriptions(num + 1)},
		} {
			t.Run(fmt.Sprintf("%d_groups/%s", num, tc.name), func(t *testing.T) {
				for _, invalidSignature := range []bool{false, true} {
					t.Run(fmt.Sprintf("invalid_signature_%t", invalidSignature), func(t *testing.T) {
						set := base.Copy()
						set.Descriptions = tc.descriptions
						if invalidSignature {
							set.Messages[num-1][0] ^= 1
						}
						// Descriptions do not affect cryptographic verification.
						valid, err := set.Verify()
						require.NoError(t, err)
						require.Equal(t, !invalidSignature, valid)
						valid, err = set.VerifyVerbosely()
						require.ErrorContains(t, "descriptions and messages have differing lengths", err)
						require.Equal(t, false, valid)
					})
				}
				set := base.Copy()
				set.Descriptions = tc.descriptions
				before := set.Copy()
				// Preserve nil versus empty metadata when checking for mutation.
				if tc.descriptions == nil {
					before.Descriptions = nil
				}
				removed, result, err := set.RemoveDuplicates()
				require.ErrorContains(t, "descriptions and messages have differing lengths", err)
				require.Equal(t, 0, removed)
				require.Equal(t, set, result)
				require.DeepEqual(t, before, set)
			})
		}
	}
}

func TestSignatureBatch_RejectsMalformedGroups(t *testing.T) {
	base := NewValidSignatureSet(t, "shape", 2)
	for _, tc := range []struct {
		name   string
		modify func(*SignatureBatch)
		err    string
	}{
		{
			name:   "missing_signature_groups",
			modify: func(s *SignatureBatch) { s.Signatures = nil },
			err:    "differing lengths",
		},
		{
			name:   "missing_public_key_groups",
			modify: func(s *SignatureBatch) { s.PublicKeys = nil },
			err:    "differing lengths",
		},
		{
			name: "missing_messages",
			modify: func(s *SignatureBatch) {
				s.Messages = nil
				s.Descriptions = nil
			},
			err: "differing lengths",
		},
		{
			name:   "short_signature_group",
			modify: func(s *SignatureBatch) { s.Signatures[1] = nil },
			err:    "differing lengths",
		},
		{
			name: "empty_group",
			modify: func(s *SignatureBatch) {
				s.Signatures[1] = nil
				s.PublicKeys[1] = nil
			},
			err: "signature group 1 is empty",
		},
		{
			name: "duplicate_with_nil_public_key",
			modify: func(s *SignatureBatch) {
				s.Join(s.Copy())
				s.PublicKeys[2][0] = nil
			},
			err: "invalid public key at batch 2, index 0",
		},
		{
			name:   "typed_nil_public_key",
			modify: func(s *SignatureBatch) { s.PublicKeys[1][0] = (*ml_dsa_87t.PublicKey)(nil) },
			err:    "invalid public key at batch 1, index 0",
		},
		{
			name:   "uninitialized_public_key",
			modify: func(s *SignatureBatch) { s.PublicKeys[1][0] = &ml_dsa_87t.PublicKey{} },
			err:    "invalid public key at batch 1, index 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, before := base.Copy(), base.Copy()
			tc.modify(set)
			tc.modify(before)
			valid, err := set.Verify()
			require.ErrorContains(t, tc.err, err)
			require.Equal(t, false, valid)
			valid, err = set.VerifySequential(t.Context())
			require.ErrorContains(t, tc.err, err)
			require.Equal(t, false, valid)
			valid, err = set.VerifyVerbosely()
			require.ErrorContains(t, tc.err, err)
			require.Equal(t, false, valid)
			removed, result, err := set.RemoveDuplicates()
			require.ErrorContains(t, tc.err, err)
			require.Equal(t, 0, removed)
			require.Equal(t, set, result)
			require.DeepEqual(t, before, set)
		})
	}
}

func TestSignatureBatch_VerifySequential(t *testing.T) {
	base := NewValidSignatureSet(t, "sequential", 2)
	// Cover multiple signatures within a group as well as multiple groups.
	base.Signatures[0] = append(base.Signatures[0], base.Signatures[0][0])
	base.PublicKeys[0] = append(base.PublicKeys[0], base.PublicKeys[0][0])
	valid, err := base.VerifySequential(t.Context())
	require.NoError(t, err)
	require.Equal(t, true, valid)
	for i := range base.Signatures {
		for j := range base.Signatures[i] {
			set := base.Copy()
			set.Signatures[i][j][0] ^= 1
			valid, err := set.VerifySequential(t.Context())
			require.NoError(t, err)
			require.Equal(t, false, valid, "every signature must verify")
		}
	}
	t.Run("stops at first invalid signature", func(t *testing.T) {
		set := base.Copy()
		set.Signatures[0][0][0] ^= 1
		// If verification continued after the first failure, parsing this later
		// signature would produce an error instead of the first signature's verdict.
		set.Signatures[1][0] = []byte{1}
		valid, err := set.VerifySequential(t.Context())
		require.NoError(t, err)
		require.Equal(t, false, valid)
		set.Signatures[0][0][0] ^= 1
		valid, err = set.VerifySequential(t.Context())
		require.ErrorContains(t, "signature must be", err)
		require.Equal(t, false, valid)
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		valid, err := base.VerifySequential(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, false, valid)
	})
	t.Run("empty", func(t *testing.T) {
		valid, err := NewSet().VerifySequential(t.Context())
		require.NoError(t, err)
		require.Equal(t, false, valid)
	})
	t.Run("nil", func(t *testing.T) {
		valid, err := (*SignatureBatch)(nil).VerifySequential(t.Context())
		require.ErrorContains(t, "nil signature set", err)
		require.Equal(t, false, valid)
	})
}

func TestCopySignatureSet_InvalidPublicKeys(t *testing.T) {
	base := NewValidSignatureSet(t, "copy", 2)
	for _, tc := range []struct {
		name string
		key  PublicKey
	}{
		{name: "nil_interface"},
		{name: "typed_nil", key: (*ml_dsa_87t.PublicKey)(nil)},
		{name: "uninitialized", key: &ml_dsa_87t.PublicKey{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := base.Copy()
			set.PublicKeys[1][0] = tc.key
			copied := set.Copy()
			require.Equal(t, nil, copied.PublicKeys[1][0])
			require.DeepEqual(t, set.PublicKeys[0], copied.PublicKeys[0])
			require.DeepEqual(t, set.Signatures, copied.Signatures)
			valid, err := copied.Verify()
			require.ErrorContains(t, "invalid public key at batch 1, index 0", err)
			require.Equal(t, false, valid)
		})
	}
}

func TestSignatureBatch_RemoveDuplicates_PreservesDescriptions(t *testing.T) {
	base := NewValidSignatureSet(t, "description", 2)
	set := base.Copy().Join(base.Copy())
	removed, result, err := set.RemoveDuplicates()
	require.NoError(t, err)
	require.Equal(t, 2, removed)
	require.DeepEqual(t, base, result)
}

func TestSignatureBatch_RemoveDuplicates(t *testing.T) {
	var keys []MLDSA87Key
	for range 100 {
		key, err := RandKey()
		assert.NoError(t, err)
		keys = append(keys, key)
	}
	tests := []struct {
		name         string
		batchCreator func() (input *SignatureBatch, output *SignatureBatch)
		want         int
	}{
		{
			name: "empty batch",
			batchCreator: func() (*SignatureBatch, *SignatureBatch) {
				return &SignatureBatch{}, &SignatureBatch{}
			},
			want: 0,
		},
		{
			name: "valid duplicates in batch",
			batchCreator: func() (*SignatureBatch, *SignatureBatch) {
				chosenKeys := keys[:20]

				msg := [32]byte{'r', 'a', 'n', 'd', 'o', 'm'}
				var signatures [][][]byte
				var pubs [][]PublicKey
				var messages [][32]byte
				for _, k := range chosenKeys {
					s, err := k.Sign(msg[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				allSigs := append(signatures, signatures...)
				allPubs := append(pubs, pubs...)
				allMsgs := append(messages, messages...)
				return &SignatureBatch{
						Signatures:   allSigs,
						PublicKeys:   allPubs,
						Messages:     allMsgs,
						Descriptions: createDescriptions(len(allMsgs)),
					}, &SignatureBatch{
						Signatures:   signatures,
						PublicKeys:   pubs,
						Messages:     messages,
						Descriptions: createDescriptions(len(allMsgs)),
					}
			},
			want: 20,
		},
		{
			name: "valid duplicates in batch with multiple messages",
			batchCreator: func() (*SignatureBatch, *SignatureBatch) {
				chosenKeys := keys[:30]

				msg := [32]byte{'r', 'a', 'n', 'd', 'o', 'm'}
				msg1 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '1'}
				msg2 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '2'}
				var signatures [][][]byte
				var messages [][32]byte
				var pubs [][]PublicKey
				for _, k := range chosenKeys[:10] {
					s, err := k.Sign(msg[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s, err := k.Sign(msg1[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s, err := k.Sign(msg2[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg2)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				allSigs := append(signatures, signatures...)
				allPubs := append(pubs, pubs...)
				allMsgs := append(messages, messages...)
				return &SignatureBatch{
						Signatures:   allSigs,
						PublicKeys:   allPubs,
						Messages:     allMsgs,
						Descriptions: createDescriptions(len(allMsgs)),
					}, &SignatureBatch{
						Signatures:   signatures,
						PublicKeys:   pubs,
						Messages:     messages,
						Descriptions: createDescriptions(len(allMsgs)),
					}
			},
			want: 30,
		},
		{
			name: "no duplicates in batch with multiple messages",
			batchCreator: func() (*SignatureBatch, *SignatureBatch) {
				chosenKeys := keys[:30]

				msg := [32]byte{'r', 'a', 'n', 'd', 'o', 'm'}
				msg1 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '1'}
				msg2 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '2'}
				var signatures [][][]byte
				var messages [][32]byte
				var pubs [][]PublicKey
				for _, k := range chosenKeys[:10] {
					s, err := k.Sign(msg[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s, err := k.Sign(msg1[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s, err := k.Sign(msg2[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg2)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				return &SignatureBatch{
						Signatures:   signatures,
						PublicKeys:   pubs,
						Messages:     messages,
						Descriptions: createDescriptions(len(messages)),
					}, &SignatureBatch{
						Signatures:   signatures,
						PublicKeys:   pubs,
						Messages:     messages,
						Descriptions: createDescriptions(len(messages)),
					}
			},
			want: 0,
		},
		{
			name: "valid duplicates and invalid duplicates in batch with multiple messages",
			batchCreator: func() (*SignatureBatch, *SignatureBatch) {
				chosenKeys := keys[:30]

				msg := [32]byte{'r', 'a', 'n', 'd', 'o', 'm'}
				msg1 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '1'}
				msg2 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '2'}
				var signatures [][][]byte
				var messages [][32]byte
				var pubs [][]PublicKey
				for _, k := range chosenKeys[:10] {
					s, err := k.Sign(msg[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s, err := k.Sign(msg1[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s, err := k.Sign(msg2[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg2)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				allSigs := append(signatures, signatures...)
				// Make it a non-unique entry
				allSigs[10] = [][]byte{make([]byte, 4627)}
				allPubs := append(pubs, pubs...)
				allMsgs := append(messages, messages...)
				// Insert it back at the end
				signatures = append(signatures, signatures[10])
				pubs = append(pubs, pubs[10])
				messages = append(messages, messages[10])
				// Zero out to expected result
				signatures[10] = [][]byte{make([]byte, 4627)}
				return &SignatureBatch{
						Signatures:   allSigs,
						PublicKeys:   allPubs,
						Messages:     allMsgs,
						Descriptions: createDescriptions(len(allMsgs)),
					}, &SignatureBatch{
						Signatures:   signatures,
						PublicKeys:   pubs,
						Messages:     messages,
						Descriptions: createDescriptions(len(allMsgs)),
					}
			},
			want: 29,
		},

		{
			name: "valid duplicates and invalid duplicates with signature,pubkey,message in batch with multiple messages",
			batchCreator: func() (*SignatureBatch, *SignatureBatch) {
				chosenKeys := keys[:30]

				msg := [32]byte{'r', 'a', 'n', 'd', 'o', 'm'}
				msg1 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '1'}
				msg2 := [32]byte{'r', 'a', 'n', 'd', 'o', 'm', '2'}
				var signatures [][][]byte
				var messages [][32]byte
				var pubs [][]PublicKey
				for _, k := range chosenKeys[:10] {
					s, err := k.Sign(msg[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s, err := k.Sign(msg1[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s, err := k.Sign(msg2[:])
					require.NoError(t, err)
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg2)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				allSigs := append(signatures, signatures...)
				// Make it a non-unique entry
				allSigs[10] = [][]byte{make([]byte, 4627)}

				allPubs := append(pubs, pubs...)
				allPubs[20] = []PublicKey{keys[len(keys)-1].PublicKey()}

				allMsgs := append(messages, messages...)
				allMsgs[29] = [32]byte{'j', 'u', 'n', 'k'}

				// Insert it back at the end
				signatures = append(signatures, signatures[10])
				pubs = append(pubs, pubs[10])
				messages = append(messages, messages[10])
				// Zero out to expected result
				signatures[10] = [][]byte{make([]byte, 4627)}

				// Insert it back at the end
				signatures = append(signatures, signatures[20])
				pubs = append(pubs, pubs[20])
				messages = append(messages, messages[20])
				// Zero out to expected result
				pubs[20] = []PublicKey{keys[len(keys)-1].PublicKey()}

				// Insert it back at the end
				signatures = append(signatures, signatures[29])
				pubs = append(pubs, pubs[29])
				messages = append(messages, messages[29])
				messages[29] = [32]byte{'j', 'u', 'n', 'k'}

				return &SignatureBatch{
						Signatures:   allSigs,
						PublicKeys:   allPubs,
						Messages:     allMsgs,
						Descriptions: createDescriptions(len(allMsgs)),
					}, &SignatureBatch{
						Signatures:   signatures,
						PublicKeys:   pubs,
						Messages:     messages,
						Descriptions: createDescriptions(len(messages)),
					}
			},
			want: 27,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, output := tt.batchCreator()
			num, res, err := input.RemoveDuplicates()
			assert.NoError(t, err)
			if num != tt.want {
				t.Errorf("RemoveDuplicates() got = %v, want %v", num, tt.want)
			}
			if !reflect.DeepEqual(res.Signatures, output.Signatures) {
				t.Errorf("RemoveDuplicates() Signatures output = %v, want %v", res.Signatures, output.Signatures)
			}
			if !reflect.DeepEqual(res.PublicKeys, output.PublicKeys) {
				t.Errorf("RemoveDuplicates() Publickeys output = %v, want %v", res.PublicKeys, output.PublicKeys)
			}
			if !reflect.DeepEqual(res.Messages, output.Messages) {
				t.Errorf("RemoveDuplicates() Messages output = %v, want %v", res.Messages, output.Messages)
			}
		})
	}
}

func NewValidSignatureSet(t *testing.T, msgBody string, num int) *SignatureBatch {
	set := &SignatureBatch{
		Signatures:   make([][][]byte, num),
		PublicKeys:   make([][]common.PublicKey, num),
		Messages:     make([][32]byte, num),
		Descriptions: make([]string, num),
	}

	for i := range num {
		priv, err := RandKey()
		require.NoError(t, err)
		pubkey := priv.PublicKey()
		msg := messageBytes(fmt.Sprintf("%s%d", msgBody, i))
		lsig1, err := priv.Sign(msg[:])
		require.NoError(t, err)
		sig := lsig1.Marshal()
		desc := fmt.Sprintf("signature of %s%d", msgBody, i)

		set.Signatures[i] = [][]byte{sig}
		set.PublicKeys[i] = []common.PublicKey{pubkey}
		set.Messages[i] = msg
		set.Descriptions[i] = desc
	}

	return set
}

func NewInvalidSignatureSet(t *testing.T, msgBody string, num int, throwErr bool) *SignatureBatch {
	set := &SignatureBatch{
		Signatures:   make([][][]byte, num),
		PublicKeys:   make([][]common.PublicKey, num),
		Messages:     make([][32]byte, num),
		Descriptions: make([]string, num),
	}

	for i := range num {
		priv, err := RandKey()
		require.NoError(t, err)
		pubkey := priv.PublicKey()
		msg := messageBytes(fmt.Sprintf("%s%d", msgBody, i))
		var sig []byte
		if throwErr {
			sig = make([]byte, fieldparams.MLDSA87SignatureLength)
		} else {
			badMsg := messageBytes("badmsg")
			lsig2, err := priv.Sign(badMsg[:])
			require.NoError(t, err)
			sig = lsig2.Marshal()
		}
		desc := fmt.Sprintf("signature of %s%d", msgBody, i)

		set.Signatures[i] = [][]byte{sig}
		set.PublicKeys[i] = []common.PublicKey{pubkey}
		set.Messages[i] = msg
		set.Descriptions[i] = desc
	}

	return set
}

func messageBytes(message string) [32]byte {
	var bytes [32]byte
	copy(bytes[:], []byte(message))
	return bytes
}
