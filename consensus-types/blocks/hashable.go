package blocks

import (
	"github.com/pkg/errors"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// errNotHashable is returned when a block or body reconstructed from protobuf
// is missing a nested object that SSZ hashing dereferences.
var errNotHashable = errors.New("block is missing a nested object required for hashing")

// checkHashable reports the first nil nested object that the generated SSZ
// hasher would dereference. Those methods allocate nothing, so hashing such
// an object panics. Blocks decoded from SSZ always have every container
// allocated; blocks built from protobuf, as the validator API and the REST
// publish path do, may not.
func checkHashable(pb proto.Message) error {
	switch p := pb.(type) {
	case *qrysmpb.BeaconBlockZond:
		if p == nil {
			return errNilBlock
		}
		return checkBodyHashable(p.Body)
	case *qrysmpb.BlindedBeaconBlockZond:
		if p == nil {
			return errNilBlock
		}
		return checkBlindedBodyHashable(p.Body)
	case *qrysmpb.BeaconBlockBodyZond:
		return checkBodyHashable(p)
	case *qrysmpb.BlindedBeaconBlockBodyZond:
		return checkBlindedBodyHashable(p)
	default:
		return nil
	}
}

func checkBodyHashable(body *qrysmpb.BeaconBlockBodyZond) error {
	if body == nil {
		return errNilBlockBody
	}
	if err := checkOperationsHashable(body.ExecutionData, body.SyncAggregate, body.ProposerSlashings,
		body.AttesterSlashings, body.Attestations, body.Deposits, body.VoluntaryExits); err != nil {
		return err
	}
	if body.ExecutionPayload == nil {
		return errors.Wrap(errNotHashable, "execution payload")
	}
	for i, w := range body.ExecutionPayload.Withdrawals {
		if w == nil {
			return errors.Wrapf(errNotHashable, "execution payload withdrawal %d", i)
		}
	}
	return nil
}

func checkBlindedBodyHashable(body *qrysmpb.BlindedBeaconBlockBodyZond) error {
	if body == nil {
		return errNilBlockBody
	}
	if err := checkOperationsHashable(body.ExecutionData, body.SyncAggregate, body.ProposerSlashings,
		body.AttesterSlashings, body.Attestations, body.Deposits, body.VoluntaryExits); err != nil {
		return err
	}
	if body.ExecutionPayloadHeader == nil {
		return errors.Wrap(errNotHashable, "execution payload header")
	}
	return nil
}

func checkOperationsHashable(
	executionData *qrysmpb.ExecutionData,
	syncAggregate *qrysmpb.SyncAggregate,
	proposerSlashings []*qrysmpb.ProposerSlashing,
	attesterSlashings []*qrysmpb.AttesterSlashing,
	attestations []*qrysmpb.Attestation,
	deposits []*qrysmpb.Deposit,
	exits []*qrysmpb.SignedVoluntaryExit,
) error {
	if executionData == nil {
		return errors.Wrap(errNotHashable, "execution data")
	}
	if syncAggregate == nil {
		return errors.Wrap(errNotHashable, "sync aggregate")
	}
	for i, s := range proposerSlashings {
		if s == nil || s.Header_1 == nil || s.Header_1.Header == nil || s.Header_2 == nil || s.Header_2.Header == nil {
			return errors.Wrapf(errNotHashable, "proposer slashing %d", i)
		}
	}
	for i, s := range attesterSlashings {
		if s == nil || !attestationDataHashable(s.Attestation_1) || !attestationDataHashable(s.Attestation_2) {
			return errors.Wrapf(errNotHashable, "attester slashing %d", i)
		}
	}
	for i, a := range attestations {
		if a == nil || !checkpointsHashable(a.Data) {
			return errors.Wrapf(errNotHashable, "attestation %d", i)
		}
	}
	for i, d := range deposits {
		if d == nil || d.Data == nil {
			return errors.Wrapf(errNotHashable, "deposit %d", i)
		}
	}
	for i, e := range exits {
		if e == nil || e.Exit == nil {
			return errors.Wrapf(errNotHashable, "voluntary exit %d", i)
		}
	}
	return nil
}

func attestationDataHashable(a *qrysmpb.IndexedAttestation) bool {
	return a != nil && checkpointsHashable(a.Data)
}

func checkpointsHashable(d *qrysmpb.AttestationData) bool {
	return d != nil && d.Source != nil && d.Target != nil
}
