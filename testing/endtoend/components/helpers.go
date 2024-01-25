package components

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/zondclient"
	"github.com/theQRL/qrysm/v4/io/file"
	e2etypes "github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

// NetworkId is the ID of the Zond chain.
const NetworkId = 1337

const timeGapPerMiningTX = 250 * time.Millisecond

const KeystorePassword = ""

var _ e2etypes.ComponentRunner = (*BeaconNodeSet)(nil)
var _ e2etypes.MultipleComponentRunners = (*BeaconNodeSet)(nil)
var _ e2etypes.ComponentRunner = (*BeaconNode)(nil)

var _ e2etypes.MultipleComponentRunners = (*ProxySet)(nil)
var _ e2etypes.EngineProxy = (*Proxy)(nil)

// WaitForBlocks waits for a certain amount of blocks to be included before returning.
func WaitForBlocks(web3 *zondclient.Client, blocksToWait uint64) error {
	block, err := web3.BlockByNumber(context.Background(), nil)
	if err != nil {
		return err
	}
	finishBlock := block.NumberU64() + blocksToWait

	for block.NumberU64() <= finishBlock {
		time.Sleep(timeGapPerMiningTX)

		block, err = web3.BlockByNumber(context.Background(), nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseJWTSecretFromFile(jwtSecretFile string) ([]byte, error) {
	enc, err := file.ReadFileAsBytes(jwtSecretFile)
	if err != nil {
		return nil, err
	}
	strData := strings.TrimSpace(string(enc))
	if len(strData) == 0 {
		return nil, fmt.Errorf("provided JWT secret in file %s cannot be empty", jwtSecretFile)
	}
	secret, err := hex.DecodeString(strings.TrimPrefix(strData, "0x"))
	if err != nil {
		return nil, err
	}
	if len(secret) < 32 {
		return nil, errors.New("provided JWT secret should be a hex string of at least 32 bytes")
	}
	return secret, nil
}
