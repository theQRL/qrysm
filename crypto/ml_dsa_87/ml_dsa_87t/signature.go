package ml_dsa_87t

import (
	"errors"
	"fmt"
	"runtime"

	pkgerrors "github.com/pkg/errors"
	"github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
	"golang.org/x/sync/errgroup"
)

var errSignatureVerificationFailed = errors.New("signature verification failed")

// Signature used in the ML-DSA-87 signature scheme.
type Signature struct {
	s *[field_params.MLDSA87SignatureLength]uint8
}

func SignatureFromBytes(sig []byte) (common.Signature, error) {
	if len(sig) != field_params.MLDSA87SignatureLength {
		return nil, fmt.Errorf("signature must be %d bytes", field_params.MLDSA87SignatureLength)
	}
	var signature [field_params.MLDSA87SignatureLength]uint8
	copy(signature[:], sig)
	return &Signature{s: &signature}, nil
}

func (s *Signature) Verify(pubKey common.PublicKey, msg []byte) bool {
	sig := *s.s
	d, err := ml_dsa_87.NewMLDSA87Descriptor()
	if err != nil {
		return false
	}
	return ml_dsa_87.Verify(msg, sig[:], pubKey.(*PublicKey).p, d)
}

func VerifySignature(sig []byte, msg [32]byte, pubKey common.PublicKey) (bool, error) {
	rSig, err := SignatureFromBytes(sig)
	if err != nil {
		return false, err
	}
	return rSig.Verify(pubKey, msg[:]), nil
}

func VerifyMultipleSignatures(sigsBatches [][][]byte, msgs [][32]byte, pubKeysBatches [][]common.PublicKey) (bool, error) {
	var (
		lenSigsBatches    = len(sigsBatches)
		lenPubKeysBatches = len(pubKeysBatches)
	)

	if len(sigsBatches) == 0 || len(pubKeysBatches) == 0 {
		return false, nil
	}

	lenMsgsBatches := len(msgs)
	if lenSigsBatches != lenPubKeysBatches || lenSigsBatches != lenMsgsBatches {
		return false, pkgerrors.Errorf("provided signatures batches, pubkeys batches and messages have differing lengths. SB: %d, PB: %d, M: %d",
			lenSigsBatches, lenPubKeysBatches, lenMsgsBatches)
	}

	maxProcs := max(runtime.GOMAXPROCS(0)-1, 1)
	grp := errgroup.Group{}
	grp.SetLimit(maxProcs)

	for i := range lenMsgsBatches {
		if len(sigsBatches[i]) != len(pubKeysBatches[i]) {
			return false, pkgerrors.Errorf("provided signatures, pubkeys have differing lengths. S: %d, P: %d, Batch: %d",
				len(sigsBatches[i]), len(pubKeysBatches[i]), i)
		}
		index := i

		for j := range sigsBatches[index] {
			jCopy := j

			grp.Go(func() error {
				ok, err := VerifySignature(sigsBatches[index][jCopy], msgs[index], pubKeysBatches[index][jCopy])
				if err != nil {
					return err
				}
				if !ok {
					return errSignatureVerificationFailed
				}

				return nil
			})
		}
	}

	if err := grp.Wait(); err != nil {
		if pkgerrors.Is(err, errSignatureVerificationFailed) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (s *Signature) Marshal() []byte {
	return s.s[:]
}

func (s *Signature) Copy() common.Signature {
	sign := *s.s
	return &Signature{s: &sign}
}
