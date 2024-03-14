package p2p_test

import (
	"fmt"
	"testing"

	"github.com/golang/snappy"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/p2p"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/crypto/hash"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/testing/assert"
)

func TestMessageIDFunction_HashesCorrectlyCapella(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	genesisValidatorsRoot := bytesutil.PadTo([]byte{'A'}, 32)
	d, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, genesisValidatorsRoot)
	assert.NoError(t, err)
	tpc := fmt.Sprintf(p2p.BlockSubnetTopicFormat, d)
	topicLen := uint64(len(tpc))
	topicLenBytes := bytesutil.Uint64ToBytesLittleEndian(topicLen)
	invalidSnappy := [32]byte{'J', 'U', 'N', 'K'}
	pMsg := &pubsubpb.Message{Data: invalidSnappy[:], Topic: &tpc}
	// Create object to hash
	combinedObj := append(params.BeaconNetworkConfig().MessageDomainInvalidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, pMsg.Data...)
	hashedData := hash.Hash(combinedObj)
	msgID := string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(genesisValidatorsRoot, pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	// Create object to hash
	combinedObj = append(params.BeaconNetworkConfig().MessageDomainValidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, validObj[:]...)
	hashedData = hash.Hash(combinedObj)
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(genesisValidatorsRoot, nMsg), "Got incorrect msg id")
}

func TestMsgID_WithNilTopic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	msg := &pubsubpb.Message{
		Data:  make([]byte, 32),
		Topic: nil,
	}

	invalid := make([]byte, 20)
	copy(invalid, "invalid")

	res := p2p.MsgID([]byte{0x01}, msg)
	assert.Equal(t, res, string(invalid))
}
