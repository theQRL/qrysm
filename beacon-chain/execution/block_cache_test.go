package execution

import (
	"math/big"
	"testing"

	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/qrysm/beacon-chain/execution/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestHashKeyFn_OK(t *testing.T) {
	hInfo := &types.HeaderInfo{
		Hash: common.HexToHash("0x0123456"),
	}

	key, err := hashKeyFn(hInfo)
	require.NoError(t, err)
	assert.Equal(t, hInfo.Hash.Hex(), key)
}

func TestHashKeyFn_InvalidObj(t *testing.T) {
	_, err := hashKeyFn("bad")
	assert.Equal(t, ErrNotAHeaderInfo, err)
}

func TestHeightKeyFn_OK(t *testing.T) {
	hInfo := &types.HeaderInfo{
		Number: big.NewInt(555),
	}

	key, err := heightKeyFn(hInfo)
	require.NoError(t, err)
	assert.Equal(t, hInfo.Number.String(), key)
}

func TestHeightKeyFn_InvalidObj(t *testing.T) {
	_, err := heightKeyFn("bad")
	assert.Equal(t, ErrNotAHeaderInfo, err)
}

func TestBlockCache_byHash(t *testing.T) {
	// TODO(now.youtrack.cloud/issue/TQ-5)
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.Eth1FollowDistance = 2048
	params.OverrideBeaconConfig(conf)
	maxCacheSize = 2 * params.BeaconConfig().Eth1FollowDistance

	cache := newHeaderCache()

	header := &types.HeaderInfo{
		Number: big.NewInt(55),
	}
	exists, _, err := cache.HeaderInfoByHash(header.Hash)
	require.NoError(t, err)
	assert.Equal(t, false, exists, "Expected block info not to exist in empty cache")

	err = cache.AddHeader(header)
	require.NoError(t, err)

	exists, fetchedInfo, err := cache.HeaderInfoByHash(header.Hash)
	require.NoError(t, err)
	assert.Equal(t, true, exists, "Expected headerInfo to exist")
	assert.Equal(t, 0, fetchedInfo.Number.Cmp(header.Number), "Expected fetched info number to be equal")
	assert.Equal(t, header.Hash, fetchedInfo.Hash, "Expected hash to be equal")
}

func TestBlockCache_byHeight(t *testing.T) {
	cache := newHeaderCache()

	header := &types.HeaderInfo{
		Number: big.NewInt(55),
	}
	exists, _, err := cache.HeaderInfoByHeight(header.Number)
	require.NoError(t, err)
	assert.Equal(t, false, exists, "Expected block info not to exist in empty cache")

	err = cache.AddHeader(header)
	require.NoError(t, err)

	exists, fetchedInfo, err := cache.HeaderInfoByHeight(header.Number)
	require.NoError(t, err)
	assert.Equal(t, true, exists, "Expected headerInfo to exist")

	assert.Equal(t, 0, fetchedInfo.Number.Cmp(header.Number), "Expected fetched info number to be equal")
	assert.Equal(t, header.Hash, fetchedInfo.Hash, "Expected hash to be equal")

}

func TestBlockCache_maxSize(t *testing.T) {
	cache := newHeaderCache()

	for i := int64(0); i < int64(maxCacheSize+10); i++ {
		header := &types.HeaderInfo{
			Number: big.NewInt(i),
			Hash:   common.Hash(bytesutil.ToBytes32(bytesutil.Bytes32(uint64(i)))),
		}
		err := cache.AddHeader(header)
		require.NoError(t, err)

	}

	assert.Equal(t, int(maxCacheSize), len(cache.hashCache.ListKeys()))
	assert.Equal(t, int(maxCacheSize), len(cache.heightCache.ListKeys()))
}
