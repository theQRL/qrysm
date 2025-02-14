package dilithium

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/theQRL/qrysm/crypto/dilithium/common"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

const TestSignature = "test signature"

func TestCopySignatureSet(t *testing.T) {
	t.Run("dilithiumt", func(t *testing.T) {
		key, err := RandKey()
		assert.NoError(t, err)
		key2, err := RandKey()
		assert.NoError(t, err)
		key3, err := RandKey()
		assert.NoError(t, err)

		message := [32]byte{'C', 'D'}
		message2 := [32]byte{'E', 'F'}
		message3 := [32]byte{'H', 'I'}

		sig := key.Sign(message[:])
		sig2 := key2.Sign(message2[:])
		sig3 := key3.Sign(message3[:])

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

func TestSignatureBatch_RemoveDuplicates(t *testing.T) {
	var keys []DilithiumKey
	for i := 0; i < 100; i++ {
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
					s := k.Sign(msg[:])
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
					s := k.Sign(msg[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s := k.Sign(msg1[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s := k.Sign(msg2[:])
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
					s := k.Sign(msg[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s := k.Sign(msg1[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s := k.Sign(msg2[:])
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
					s := k.Sign(msg[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s := k.Sign(msg1[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s := k.Sign(msg2[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg2)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				allSigs := append(signatures, signatures...)
				// Make it a non-unique entry
				allSigs[10] = [][]byte{make([]byte, 4595)}
				allPubs := append(pubs, pubs...)
				allMsgs := append(messages, messages...)
				// Insert it back at the end
				signatures = append(signatures, signatures[10])
				pubs = append(pubs, pubs[10])
				messages = append(messages, messages[10])
				// Zero out to expected result
				signatures[10] = [][]byte{make([]byte, 4595)}
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
					s := k.Sign(msg[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[10:20] {
					s := k.Sign(msg1[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg1)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				for _, k := range chosenKeys[20:30] {
					s := k.Sign(msg2[:])
					signatures = append(signatures, [][]byte{s.Marshal()})
					messages = append(messages, msg2)
					pubs = append(pubs, []PublicKey{k.PublicKey()})
				}
				allSigs := append(signatures, signatures...)
				// Make it a non-unique entry
				allSigs[10] = [][]byte{make([]byte, 4595)}

				allPubs := append(pubs, pubs...)
				allPubs[20] = []PublicKey{keys[len(keys)-1].PublicKey()}

				allMsgs := append(messages, messages...)
				allMsgs[29] = [32]byte{'j', 'u', 'n', 'k'}

				// Insert it back at the end
				signatures = append(signatures, signatures[10])
				pubs = append(pubs, pubs[10])
				messages = append(messages, messages[10])
				// Zero out to expected result
				signatures[10] = [][]byte{make([]byte, 4595)}

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

	for i := 0; i < num; i++ {
		priv, err := RandKey()
		require.NoError(t, err)
		pubkey := priv.PublicKey()
		msg := messageBytes(fmt.Sprintf("%s%d", msgBody, i))
		sig := priv.Sign(msg[:]).Marshal()
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

	for i := 0; i < num; i++ {
		priv, err := RandKey()
		require.NoError(t, err)
		pubkey := priv.PublicKey()
		msg := messageBytes(fmt.Sprintf("%s%d", msgBody, i))
		var sig []byte
		if throwErr {
			sig = make([]byte, 4595)
		} else {
			badMsg := messageBytes("badmsg")
			sig = priv.Sign(badMsg[:]).Marshal()
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
