package interop

import (
	"math"
	"math/big"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/go-qrl/core"
	"github.com/theQRL/go-qrl/params"
	clparams "github.com/theQRL/qrysm/config/params"
	contracts "github.com/theQRL/qrysm/contracts/deposit"
)

// defaultMinerAddress is used to send deposits and test transactions in the e2e test.
// This account is given a large initial balance in the genesis block in test setups.
const defaultTestChainId int64 = 1337
const defaultMixhash = "0x0000000000000000000000000000000000000000000000000000000000000000"
const defaultParenthash = "0x0000000000000000000000000000000000000000000000000000000000000000"
const defaultTestAccountBalance = "80000000000000000000000000"

var defaultTestAccountAddress, _ = common.NewAddressFromString("Qaf84bc06703edfc371a0177ac8b482622d5ad24204145f01746cb381dcd546c53b8825839cc61bfc1fc3d78bc560c7bb7a9895432e1e87435474a1bc5a2e1200")
var defaultCoinbase, _ = common.NewAddressFromString("Q000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")

// DefaultDepositContractStorage is the storage the deposit contract's constructor writes: the
// zero hashes of the empty deposit trie. A genesis allocation does not run the constructor, so
// they are precomputed here.
//
// The QRVM storage word is 64 bytes and Hyperion packs two bytes32 values into it, so
// zero_hashes[2n] and zero_hashes[2n+1] share slot 0x11+n, with zero_hashes[2n+1] in the
// high-order half (branch takes slots 0x00 to 0x0f and deposit_count slot 0x10).
// zero_hashes[0] is zero. This is not the layout of the Ethereum contract, which has one
// bytes32 per slot. TestGqrlTestnetGenesis_DepositContractMatchesConstructor checks these
// values against a contract created by its constructor.
var DefaultDepositContractStorage = map[string]string{
	"0x0000000000000000000000000000000000000000000000000000000000000011": "0xf5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b0000000000000000000000000000000000000000000000000000000000000000",
	"0x0000000000000000000000000000000000000000000000000000000000000012": "0xc78009fdf07fc56a11f122370658a353aaa542ed63e44c4bc15ff4cd105ab33cdb56114e00fdd4c1f85c892bf35ac9a89289aaecb1ebd0a96cde606a748b5d71",
	"0x0000000000000000000000000000000000000000000000000000000000000013": "0x9efde052aa15429fae05bad4d0b1d7c64da64d03d7a1854a588c2cb8430c0d30536d98837f2dd165a55d5eeae91485954472d56f246df256bf3cae19352a123c",
	"0x0000000000000000000000000000000000000000000000000000000000000014": "0x87eb0ddba57e35f6d286673802a4af5975e22506c7cf4c64bb6be5ee11527f2cd88ddfeed400a8755596b21942c1497e114c302e6118290f91e6772976041fa1",
	"0x0000000000000000000000000000000000000000000000000000000000000015": "0x506d86582d252405b840018792cad2bf1259f1ef5aa5f887e13cb2f0094f51e126846476fd5fc54a5d43385167c95144f2643f533cc85bb9d16b782f8d7db193",
	"0x0000000000000000000000000000000000000000000000000000000000000016": "0x6cf04127db05441cd833107a52be852868890e4317e6a02ab47683aa75964220ffff0ad7e659772f9534c195c815efc4014ef1e1daed4404c06385d11192e92b",
	"0x0000000000000000000000000000000000000000000000000000000000000017": "0xdf6af5f5bbdb6be9ef8aa618e4bf8073960867171e29676f8b284dea6a08a85eb7d05f875f140027ef5118a2247bbb84ce8f2f0f1123623085daf7960c329f5f",
	"0x0000000000000000000000000000000000000000000000000000000000000018": "0xd49a7502ffcfb0340b1d7885688500ca308161a7f96b62df9d083b71fcc8f2bbb58d900f5e182e3c50ef74969ea16c7726c549757cc23523c369587da7293784",
	"0x0000000000000000000000000000000000000000000000000000000000000019": "0x8d0d63c39ebade8509e0ae3c9c3876fb5fa112be18f905ecacfecb92057603ab8fe6b1689256c0d385f42f5bbe2027a22c1996e110ba97c171d3e5948de92beb",
	"0x000000000000000000000000000000000000000000000000000000000000001a": "0xf893e908917775b62bff23294dbbe3a1cd8e6cc1c35b4801887b646a6f81f17f95eec8b2e541cad4e91de38385f2e046619f54496c2382cb6cacd5b98c26f5a4",
	"0x000000000000000000000000000000000000000000000000000000000000001b": "0x8a8d7fe3af8caa085a7639a832001457dfb9128a8061142ad0335629ff23ff9ccddba7b592e3133393c16194fac7431abf2f5485ed711db282183c819e08ebaa",
	"0x000000000000000000000000000000000000000000000000000000000000001c": "0xe71f0aa83cc32edfbefa9f4d3e0174ca85182eec9f3a09f6a6c0df6377a510d7feb3c337d7a51a6fbf00b9e34c52e1c9195c969bd4e7a0bfd51d5c5bed9c1167",
	"0x000000000000000000000000000000000000000000000000000000000000001d": "0x21352bfecbeddde993839f614c3dac0a3ee37543f9b412b16199dc158e23b54431206fa80a50bb6abe29085058f16212212a60eec8f049fecb92d8c8e0a84bc0",
	"0x000000000000000000000000000000000000000000000000000000000000001e": "0x7cdd2986268250628d0c10e385c58c6191e6fbe05191bcc04f133f2cea72c1c4619e312724bb6d7c3153ed9de791d764a366b389af13c58bf8a8d90481a46765",
	"0x000000000000000000000000000000000000000000000000000000000000001f": "0x8869ff2c22b28cc10510d9853292803328be4fb0e80495e8bb8d271f5b889636848930bd7ba8cac54661072113fb278869e07bb8587f91392933374d017bcbe1",
	"0x0000000000000000000000000000000000000000000000000000000000000020": "0x985e929f70af28d0bdd1a90a808f977f597c7c778c489e98d3bd8910d31ac0f7b5fe28e79f1b850f8658246ce9b6a1e7b49fc06db7143e8fe0b4f2b0c5523a5c",
}

var bigz = big.NewInt(0)
var testAccountBalance = big.NewInt(0)

// GqrlTestnetGenesis creates a genesis.json for execution clients with a set of defaults suitable for ephemeral testnets,
// like in an e2e test. The parameters are minimal but the full value is returned unmarshaled so that it can be
// customized as desired.
func GqrlTestnetGenesis(genesisTime uint64, cfg *clparams.BeaconChainConfig) *core.Genesis {
	cc := &params.ChainConfig{
		ChainID: big.NewInt(defaultTestChainId),
	}
	da := defaultDepositContractAllocation(cfg.DepositContractAddress)
	ma := minerAllocation()

	return &core.Genesis{
		Config:    cc,
		Timestamp: genesisTime,
		// NOTE(rgeraldes24): required by the genesis generation on the beacon node side
		// during the e2e tests
		ExtraData: make([]byte, 32),
		GasLimit:  params.MaxGasLimit,
		Mixhash:   common.HexToHash(defaultMixhash),
		Coinbase:  defaultCoinbase,
		Alloc: core.GenesisAlloc{
			da.Address: da.Account,
			ma.Address: ma.Account,
		},
		ParentHash: common.HexToHash(defaultParenthash),
	}
}

type depositAllocation struct {
	Address common.Address
	Account core.GenesisAccount
}

func minerAllocation() depositAllocation {
	return depositAllocation{
		Address: defaultTestAccountAddress,
		Account: core.GenesisAccount{
			Balance: testAccountBalance,
		},
	}
}

func defaultDepositContractAllocation(contractAddress string) depositAllocation {
	s := make(map[common.Hash]common.StorageValue64)
	for k, v := range DefaultDepositContractStorage {
		s[common.HexToHash(k)] = common.HexToStorageValue64(v)
	}
	codeBytes, err := hexutil.Decode(contracts.DepositContractRuntimeCodeHex())
	if err != nil {
		panic(err) // lint:nopanic
	}
	contractAddr, err := common.NewAddressFromString(contractAddress)
	if err != nil {
		panic(err) // lint:nopanic
	}
	return depositAllocation{
		Address: contractAddr,
		Account: core.GenesisAccount{
			Code:    codeBytes,
			Storage: s,
			Balance: bigz,
			Nonce:   deterministicNonce(0),
		},
	}
}

func deterministicNonce(i uint64) uint64 {
	return math.MaxUint64/2 + i
}

func init() {
	err := testAccountBalance.UnmarshalText([]byte(defaultTestAccountBalance))
	if err != nil {
		panic(err)
	}
}
