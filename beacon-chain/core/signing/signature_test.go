package signing_test

import (
	"testing"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestVerifyRegistrationSignature(t *testing.T) {
	sk, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	reg := &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("fee"), fieldparams.FeeRecipientLength),
		GasLimit:     123456,
		Timestamp:    uint64(time.Now().Unix()),
		Pubkey:       sk.PublicKey().Marshal(),
	}
	d := params.BeaconConfig().DomainApplicationBuilder
	domain, err := signing.ComputeDomain(d, nil, nil)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(reg, domain)
	require.NoError(t, err)
	sk.Sign(sr[:]).Marshal()

	sReg := &qrysmpb.SignedValidatorRegistrationV1{
		Message:   reg,
		Signature: sk.Sign(sr[:]).Marshal(),
	}
	require.NoError(t, signing.VerifyRegistrationSignature(sReg))

	sReg.Signature = []byte("bad")
	require.ErrorIs(t, signing.VerifyRegistrationSignature(sReg), signing.ErrSigFailedToVerify)

	sReg.Message = nil
	require.ErrorIs(t, signing.VerifyRegistrationSignature(sReg), signing.ErrNilRegistration)
}
