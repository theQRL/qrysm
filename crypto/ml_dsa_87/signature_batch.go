package ml_dsa_87

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/ml_dsa_87t"
)

const (
	maxVerboseSignatureFailures   = 8
	maxVerboseSignatureErrorBytes = 4096
	verboseSignaturePrefixBytes   = 16
	maxVerboseSignatureTextBytes  = 128
)

// SignatureBatch refers to the defined set of
// signatures and its respective public keys and
// messages required to verify it.
type SignatureBatch struct {
	Signatures [][][]byte
	PublicKeys [][]PublicKey
	Messages   [][32]byte
	// Descriptions must contain one label per message for VerifyVerbosely and
	// RemoveDuplicates. Verify does not use this metadata.
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

// Verify checks the current signature set in parallel.
func (s *SignatureBatch) Verify() (bool, error) {
	return VerifyMultipleSignatures(s.Signatures, s.Messages, s.PublicKeys)
}

// VerifySequential checks signatures in order, stopping on the first failure or
// cancellation. Callers that already limit concurrent requests can use this to
// avoid starting another worker pool for every request. Descriptions are unused.
func (s *SignatureBatch) VerifySequential(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s == nil {
		return false, errors.New("nil signature set")
	}
	if err := ml_dsa_87t.ValidateSignatureBatch(s.Signatures, s.Messages, s.PublicKeys); err != nil {
		return false, err
	}
	if len(s.Signatures) == 0 {
		return false, nil
	}
	for i, msg := range s.Messages {
		for j, signature := range s.Signatures[i] {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			valid, err := VerifySignature(signature, msg, s.PublicKeys[i][j])
			if err != nil || !valid {
				return false, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SignatureBatch) validateDescriptions() error {
	if len(s.Descriptions) != len(s.Messages) {
		return errors.Errorf("descriptions and messages have differing lengths. D: %d, M: %d", len(s.Descriptions), len(s.Messages))
	}
	return nil
}

// VerifyVerbosely verifies signatures in parallel once and reports a bounded
// sample of failures. Diagnostics contain short byte prefixes instead of full
// signatures and public keys, and the resulting error is capped at 4 KiB.
func (s *SignatureBatch) VerifyVerbosely() (bool, error) {
	if s == nil {
		return false, errors.New("nil signature set")
	}
	if err := s.validateDescriptions(); err != nil {
		return false, err
	}
	type failure struct {
		batchIndex, signatureIndex int
		err                        error
	}
	var mu sync.Mutex
	var failures [maxVerboseSignatureFailures]failure
	invalidCount := 0
	valid, err := ml_dsa_87t.VerifyMultipleSignaturesWithReporter(s.Signatures, s.Messages, s.PublicKeys, func(i, j int, err error) {
		mu.Lock()
		defer mu.Unlock()
		if invalidCount < len(failures) {
			failures[invalidCount] = failure{batchIndex: i, signatureIndex: j, err: err}
		}
		invalidCount++
	})
	if invalidCount == 0 {
		return valid, err
	}

	var errmsg strings.Builder
	errmsg.WriteString("some signatures are invalid. details:")
	reported := 0
	for _, f := range failures[:min(invalidCount, len(failures))] {
		i, j := f.batchIndex, f.signatureIndex
		sig := s.Signatures[i][j]
		pub := s.PublicKeys[i][j].Marshal()
		detail := fmt.Sprintf("\nsignature '%s' is invalid. batch: %d, index: %d,"+
			" signature prefix: 0x%x (%d bytes), public key prefix: 0x%x (%d bytes), message: 0x%x",
			truncateVerboseSignatureText(s.Descriptions[i]), i, j,
			sig[:min(len(sig), verboseSignaturePrefixBytes)], len(sig),
			pub[:min(len(pub), verboseSignaturePrefixBytes)], len(pub), s.Messages[i])
		if f.err != nil {
			detail += ", error: " + truncateVerboseSignatureText(f.err.Error())
		}
		// Reserve enough space for the omitted-count summary, including its integer.
		if errmsg.Len()+len(detail) > maxVerboseSignatureErrorBytes-128 {
			break
		}
		errmsg.WriteString(detail)
		reported++
	}
	if reported < invalidCount {
		_, _ = fmt.Fprintf(&errmsg, "\n%d additional invalid signatures omitted.", invalidCount-reported)
	}

	return false, errors.New(errmsg.String())
}

func truncateVerboseSignatureText(text string) string {
	if len(text) > maxVerboseSignatureTextBytes {
		return text[:maxVerboseSignatureTextBytes] + "..."
	}
	return text
}

// Copy the attached signature batch and return it
// to the caller. Uninitialized public keys are copied as nil entries, which
// verification will reject.
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
			if s.PublicKeys[i][j] != nil {
				pubkeys[i][j] = s.PublicKeys[i][j].Copy()
			}
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
	// Validate the entire batch before comparing keys or compacting any slices.
	if err := ml_dsa_87t.ValidateSignatureBatch(s.Signatures, s.Messages, s.PublicKeys); err != nil {
		return 0, s, err
	}
	if err := s.validateDescriptions(); err != nil {
		return 0, s, err
	}
	if len(s.Signatures) == 0 {
		return 0, s, nil
	}

	msgMap := make(map[string][]int)
	duplicateSet := make(map[int]bool)

loop:
	for i := 0; i < len(s.Messages); i++ {
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
