package client

import (
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestUpdateLogAggregateStats(t *testing.T) {
	v := &validator{
		logValidatorBalances: true,
		startBalances:        make(map[[field_params.DilithiumPubkeyLength]byte]uint64),
		prevBalance:          make(map[[field_params.DilithiumPubkeyLength]byte]uint64),
		voteStats: voteStats{
			startEpoch: 0, // this would otherwise have been previously set in LogValidatorGainsAndLosses()
		},
	}

	pubKeyBytes := [][field_params.DilithiumPubkeyLength]byte{
		bytesutil.ToBytes2592([]byte("000000000000000000000000000000000000000012345678")),
		bytesutil.ToBytes2592([]byte("000000000000000000000000000000000000000099999999")),
		bytesutil.ToBytes2592([]byte("000000000000000000000000000000000000000055555555")),
	}

	v.startBalances[pubKeyBytes[0]] = uint64(32100000000)
	v.startBalances[pubKeyBytes[1]] = uint64(32200000000)
	v.startBalances[pubKeyBytes[2]] = uint64(33000000000)

	// 7 attestations included
	responses := []*zondpb.ValidatorPerformanceResponse{
		{
			PublicKeys: [][]byte{
				bytesutil.FromBytes2592(pubKeyBytes[0]),
				bytesutil.FromBytes2592(pubKeyBytes[1]),
				bytesutil.FromBytes2592(pubKeyBytes[2]),
			},
			CorrectlyVotedHead:   []bool{false, true, false},
			CorrectlyVotedSource: []bool{false, true, true},
			CorrectlyVotedTarget: []bool{false, false, true}, // test one fast target that is included
		},
		{
			PublicKeys: [][]byte{
				bytesutil.FromBytes2592(pubKeyBytes[0]),
				bytesutil.FromBytes2592(pubKeyBytes[1]),
				bytesutil.FromBytes2592(pubKeyBytes[2]),
			},
			CorrectlyVotedHead:   []bool{true, true, true},
			CorrectlyVotedSource: []bool{true, true, true},
			CorrectlyVotedTarget: []bool{true, true, true},
		},
		{
			PublicKeys: [][]byte{
				bytesutil.FromBytes2592(pubKeyBytes[0]),
				bytesutil.FromBytes2592(pubKeyBytes[1]),
				bytesutil.FromBytes2592(pubKeyBytes[2]),
			},
			CorrectlyVotedHead:   []bool{true, false, true},
			CorrectlyVotedSource: []bool{true, false, true},
			CorrectlyVotedTarget: []bool{false, false, true},
		},
	}

	v.prevBalance[pubKeyBytes[0]] = uint64(33200000000)
	v.prevBalance[pubKeyBytes[1]] = uint64(33300000000)
	v.prevBalance[pubKeyBytes[2]] = uint64(31000000000)

	var hook *logTest.Hook

	for i, val := range responses {
		if i == len(responses)-1 { // Handle last log.
			hook = logTest.NewGlobal()
		}

		v.UpdateLogAggregateStats(val, params.BeaconConfig().SlotsPerEpoch*primitives.Slot(i+1))
	}

	require.LogsContain(t, hook, "msg=\"Previous epoch aggregated voting summary\" attestationInclusionPct=\"67%\" "+
		"averageInactivityScore=0 correctlyVotedHeadPct=\"100%\" correctlyVotedSourcePct=\"100%\" correctlyVotedTargetPct=\"50%\" epoch=2")
	require.LogsContain(t, hook, "msg=\"Vote summary since launch\" attestationsInclusionPct=\"78%\" "+
		"correctlyVotedHeadPct=\"86%\" correctlyVotedSourcePct=\"100%\" "+
		"correctlyVotedTargetPct=\"71%\" numberOfEpochs=3 pctChangeCombinedBalance=\"0.20555%\"")
}
