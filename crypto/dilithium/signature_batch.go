package dilithium

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/pkg/errors"
)

// SignatureBatch refers to the defined set of
// signatures and its respective public keys and
// messages required to verify it.
type SignatureBatch struct {
	Signatures   [][][]byte
	PublicKeys   [][]PublicKey
	Messages     [][32]byte
	Descriptions []string
}

// NewSet constructs an empty signature batch object.
func NewSet() *SignatureBatch {
	return &SignatureBatch{
		Signatures:   [][][]byte{},
		PublicKeys:   [][]PublicKey{},
		Messages:     [][32]byte{},
		Descriptions: []string{},
	}
}

// Join merges the provided signature batch to out current one.
func (s *SignatureBatch) Join(set *SignatureBatch) *SignatureBatch {
	s.Signatures = append(s.Signatures, set.Signatures...)
	s.PublicKeys = append(s.PublicKeys, set.PublicKeys...)
	s.Messages = append(s.Messages, set.Messages...)
	s.Descriptions = append(s.Descriptions, set.Descriptions...)
	return s
}

// Verify the current signature batch using the batch verify algorithm.
func (s *SignatureBatch) Verify() (bool, error) {
	return VerifyMultipleSignatures(s.Signatures, s.Messages, s.PublicKeys)
}

// VerifyVerbosely verifies signatures as a whole at first, if fails, fallback
// to verify each single signature to identify invalid ones.
func (s *SignatureBatch) VerifyVerbosely() (bool, error) {
	valid, err := s.Verify()
	if err != nil || valid {
		return valid, err
	}

	// if signature batch is invalid, we then verify signatures one by one.

	errmsg := "some signatures are invalid. details:"

	for i, msg := range s.Messages {
		for j, sig := range s.Signatures[i] {
			pubKey := s.PublicKeys[i][j]

			valid, err := VerifySignature(sig, msg, pubKey)
			if !valid {
				desc := s.Descriptions[i]
				if err != nil {
					errmsg += fmt.Sprintf("\nsignature '%s' is invalid."+
						" signature: 0x%s, public key: 0x%s, message: 0x%v, error: %v",
						desc, hex.EncodeToString(sig), hex.EncodeToString(pubKey.Marshal()),
						hex.EncodeToString(msg[:]), err)
				} else {
					errmsg += fmt.Sprintf("\nsignature '%s' is invalid."+
						" signature: 0x%s, public key: 0x%s, message: 0x%v",
						desc, hex.EncodeToString(sig), hex.EncodeToString(pubKey.Marshal()),
						hex.EncodeToString(msg[:]))
				}
			}
		}
	}

	return false, errors.Errorf(errmsg)
}

// Copy the attached signature batch and return it
// to the caller.
func (s *SignatureBatch) Copy() *SignatureBatch {
	signatures := make([][][]byte, len(s.Signatures))
	pubkeys := make([][]PublicKey, len(s.PublicKeys))
	messages := make([][32]byte, len(s.Messages))
	descriptions := make([]string, len(s.Descriptions))
	for i := range s.Signatures {
		signatures[i] = make([][]byte, len(s.Signatures[i]))
		for j := range s.Signatures[i] {
			sig := make([]byte, len(s.Signatures[i][j]))
			copy(sig, s.Signatures[i][j])
			signatures[i][j] = sig
		}
	}
	for i := range s.PublicKeys {
		pubkeys[i] = make([]PublicKey, len(s.PublicKeys[i]))
		for j := range s.PublicKeys[i] {
			pubkeys[i][j] = s.PublicKeys[i][j].Copy()
		}
	}
	for i := range s.Messages {
		copy(messages[i][:], s.Messages[i][:])
	}
	copy(descriptions, s.Descriptions)
	return &SignatureBatch{
		Signatures:   signatures,
		PublicKeys:   pubkeys,
		Messages:     messages,
		Descriptions: descriptions,
	}
}

func (s *SignatureBatch) RemoveDuplicates() (int, *SignatureBatch, error) {
	if len(s.Signatures) == 0 || len(s.PublicKeys) == 0 || len(s.Messages) == 0 {
		return 0, s, nil
	}

	if len(s.Signatures) != len(s.PublicKeys) || len(s.Signatures) != len(s.Messages) {
		return 0, s, errors.Errorf("mismatch number of signatures batches, publickeys batches and messages in signature batch. "+
			"Signatures Batches %d, Public Keys Batches %d , Messages %d", s.Signatures, s.PublicKeys, s.Messages)
	}

	msgMap := make(map[string][]int)
	duplicateSet := make(map[int]bool)

loop:
	for i := 0; i < len(s.Messages); i++ {
		if len(s.Signatures[i]) != len(s.PublicKeys[i]) {
			return 0, s, errors.Errorf("mismatch number of signatures and publickeys in signature batch[%d]. "+
				"Signatures %d, Public Keys %d", i, len(s.Signatures[i]), len(s.PublicKeys[i]))
		}

		if indices, ok := msgMap[string(s.Messages[i][:])]; ok {
		loop2:
			for _, msgIdx := range indices {
				if len(s.PublicKeys[msgIdx]) != len(s.PublicKeys[i]) {
					continue loop2
				}

				for j := 0; j < len(s.PublicKeys[msgIdx]); j++ {
					if !s.PublicKeys[msgIdx][j].Equals(s.PublicKeys[i][j]) {
						continue loop2
					}

					if !bytes.Equal(s.Signatures[msgIdx][j], s.Signatures[i][j]) {
						continue loop2
					}
				}

				duplicateSet[i] = true
				continue loop
			}
		}
		msgMap[string(s.Messages[i][:])] = append(msgMap[string(s.Messages[i][:])], i)
	}

	sigs := s.Signatures[:0]
	pubs := s.PublicKeys[:0]
	msgs := s.Messages[:0]
	descs := s.Descriptions[:0]

	for i := 0; i < len(s.Signatures); i++ {
		if duplicateSet[i] {
			continue
		}
		sigs = append(sigs, s.Signatures[i])
		pubs = append(pubs, s.PublicKeys[i])
		msgs = append(msgs, s.Messages[i])
		descs = append(descs, s.Descriptions[i])
	}

	s.Signatures = sigs
	s.PublicKeys = pubs
	s.Messages = msgs
	s.Descriptions = descs

	return len(duplicateSet), s, nil
}
