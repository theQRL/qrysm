package interop

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	b "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
)

var errUnsupportedVersion = errors.New("schema version not supported by PremineGenesisConfig")

type PremineGenesisConfig struct {
	GenesisTime    uint64
	NVals          uint64
	Version        int          // as in "github.com/theQRL/qrysm/runtime/version"
	GB             *types.Block // gqrl genesis block
	depositEntries *depositEntries
}

type depositEntries struct {
	dds   []*qrysmpb.Deposit_Data
	roots [][]byte
}

type PremineGenesisOpt func(*PremineGenesisConfig)

func WithDepositData(dds []*qrysmpb.Deposit_Data, roots [][]byte) PremineGenesisOpt {
	return func(cfg *PremineGenesisConfig) {
		cfg.depositEntries = &depositEntries{
			dds:   dds,
			roots: roots,
		}
	}
}

// NewPreminedGenesis creates a genesis BeaconState at the given fork version, suitable for using as an e2e genesis.
func NewPreminedGenesis(ctx context.Context, t, nvals uint64, version int, gb *types.Block, opts ...PremineGenesisOpt) (state.BeaconState, error) {
	cfg := &PremineGenesisConfig{
		GenesisTime: t,
		NVals:       nvals,
		Version:     version,
		GB:          gb,
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg.prepare(ctx)
}

func (s *PremineGenesisConfig) prepare(ctx context.Context) (state.BeaconState, error) {
	switch s.Version {
	case version.Zond:
	default:
		return nil, errors.Wrapf(errUnsupportedVersion, "version=%s", version.String(s.Version))
	}

	st, err := s.empty()
	if err != nil {
		return nil, err
	}
	if err = s.processDeposits(ctx, st); err != nil {
		return nil, err
	}
	if err = s.populate(st); err != nil {
		return nil, err
	}

	return st, nil
}

func (s *PremineGenesisConfig) empty() (state.BeaconState, error) {
	var e state.BeaconState
	var err error

	bRoots := make([][]byte, fieldparams.BlockRootsLength)
	for i := range bRoots {
		bRoots[i] = bytesutil.PadTo([]byte{}, 32)
	}
	sRoots := make([][]byte, fieldparams.StateRootsLength)
	for i := range sRoots {
		sRoots[i] = bytesutil.PadTo([]byte{}, 32)
	}
	mixes := make([][]byte, fieldparams.RandaoMixesLength)
	for i := range mixes {
		mixes[i] = bytesutil.PadTo([]byte{}, 32)
	}

	switch s.Version {
	case version.Zond:
		e, err = state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
			BlockRoots:       bRoots,
			StateRoots:       sRoots,
			RandaoMixes:      mixes,
			Balances:         []uint64{},
			InactivityScores: []uint64{},
			Validators:       []*qrysmpb.Validator{},
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, errUnsupportedVersion
	}
	if err = e.SetSlot(0); err != nil {
		return nil, err
	}
	if err = e.SetJustificationBits([]byte{0}); err != nil {
		return nil, err
	}
	if err = e.SetHistoricalRoots([][]byte{}); err != nil {
		return nil, err
	}
	zcp := &qrysmpb.Checkpoint{
		Epoch: 0,
		Root:  params.BeaconConfig().ZeroHash[:],
	}
	if err = e.SetPreviousJustifiedCheckpoint(zcp); err != nil {
		return nil, err
	}
	if err = e.SetCurrentJustifiedCheckpoint(zcp); err != nil {
		return nil, err
	}
	if err = e.SetFinalizedCheckpoint(zcp); err != nil {
		return nil, err
	}
	if err = e.SetExecutionDataVotes([]*qrysmpb.ExecutionData{}); err != nil {
		return nil, err
	}
	return e.Copy(), nil
}

func (s *PremineGenesisConfig) processDeposits(ctx context.Context, g state.BeaconState) error {
	deposits, err := s.deposits()
	if err != nil {
		return err
	}
	if err = s.setExecutionData(g); err != nil {
		return err
	}
	if _, err = helpers.UpdateGenesisExecutionData(g, deposits, g.ExecutionData()); err != nil {
		return err
	}
	_, err = b.ProcessPreGenesisDeposits(ctx, g, deposits)
	if err != nil {
		return errors.Wrap(err, "could not process validator deposits")
	}
	return nil
}

func (s *PremineGenesisConfig) deposits() ([]*qrysmpb.Deposit, error) {
	if s.depositEntries == nil {
		prv, pub, err := s.keys()
		if err != nil {
			return nil, err
		}
		dds, roots, err := DepositDataFromKeys(prv, pub)
		if err != nil {
			return nil, errors.Wrap(err, "could not generate deposit data from keys")
		}
		s.depositEntries = &depositEntries{
			dds:   dds,
			roots: roots,
		}
	}
	t, err := trie.GenerateTrieFromItems(s.depositEntries.roots, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate Merkle trie for deposit proofs")
	}
	deposits, err := GenerateDepositsFromData(s.depositEntries.dds, t)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate deposits from the deposit data provided")
	}
	return deposits, nil
}

func (s *PremineGenesisConfig) keys() ([]ml_dsa_87.MLDSA87Key, []ml_dsa_87.PublicKey, error) {
	prv, pub, err := DeterministicallyGenerateKeys(0, s.NVals)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not deterministically generate keys for %d validators", s.NVals)
	}
	return prv, pub, nil
}

func (s *PremineGenesisConfig) setExecutionData(g state.BeaconState) error {
	if err := g.SetExecutionDepositIndex(0); err != nil {
		return err
	}
	dr, err := emptyDepositRoot()
	if err != nil {
		return err
	}
	return g.SetExecutionData(&qrysmpb.ExecutionData{DepositRoot: dr[:], BlockHash: s.GB.Hash().Bytes()})
}

func emptyDepositRoot() ([32]byte, error) {
	t, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return [32]byte{}, err
	}
	return t.HashTreeRoot()
}

func (s *PremineGenesisConfig) populate(g state.BeaconState) error {
	if err := g.SetGenesisTime(s.GenesisTime); err != nil {
		return err
	}
	if err := s.setGenesisValidatorsRoot(g); err != nil {
		return err
	}
	if err := s.setFork(g); err != nil {
		return err
	}
	rao := nSetRoots(uint64(params.BeaconConfig().EpochsPerHistoricalVector), s.GB.Hash().Bytes())
	if err := g.SetRandaoMixes(rao); err != nil {
		return err
	}
	if err := g.SetBlockRoots(nZeroRoots(uint64(params.BeaconConfig().SlotsPerHistoricalRoot))); err != nil {
		return err
	}
	if err := g.SetStateRoots(nZeroRoots(uint64(params.BeaconConfig().SlotsPerHistoricalRoot))); err != nil {
		return err
	}
	if err := g.SetSlashings(make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)); err != nil {
		return err
	}
	if err := s.setLatestBlockHeader(g); err != nil {
		return err
	}
	if err := s.setInactivityScores(g); err != nil {
		return err
	}
	if err := s.setCurrentEpochParticipation(g); err != nil {
		return err
	}
	if err := s.setPrevEpochParticipation(g); err != nil {
		return err
	}
	if err := s.setSyncCommittees(g); err != nil {
		return err
	}
	if err := s.setExecutionPayload(g); err != nil {
		return err
	}

	// For pre-mined genesis, we want to keep the deposit root set to the root of an empty trie.
	// This needs to be set again because the methods used by processDeposits mutate the state's executionData.
	return s.setExecutionData(g)
}

func (s *PremineGenesisConfig) setGenesisValidatorsRoot(g state.BeaconState) error {
	vroot, err := stateutil.ValidatorRegistryRoot(g.Validators())
	if err != nil {
		return err
	}
	return g.SetGenesisValidatorsRoot(vroot[:])
}

func (s *PremineGenesisConfig) setFork(g state.BeaconState) error {
	var pv, cv []byte
	switch s.Version {
	case version.Zond:
		pv, cv = params.BeaconConfig().GenesisForkVersion, params.BeaconConfig().GenesisForkVersion
	default:
		return errUnsupportedVersion
	}
	fork := &qrysmpb.Fork{
		PreviousVersion: pv,
		CurrentVersion:  cv,
		Epoch:           0,
	}
	return g.SetFork(fork)
}

func (s *PremineGenesisConfig) setInactivityScores(g state.BeaconState) error {
	scores, err := g.InactivityScores()
	if err != nil {
		return err
	}
	scoresMissing := len(g.Validators()) - len(scores)
	if scoresMissing > 0 {
		for range scoresMissing {
			scores = append(scores, 0)
		}
	}
	return g.SetInactivityScores(scores)
}

func (s *PremineGenesisConfig) setCurrentEpochParticipation(g state.BeaconState) error {
	p, err := g.CurrentEpochParticipation()
	if err != nil {
		return err
	}
	missing := len(g.Validators()) - len(p)
	if missing > 0 {
		for range missing {
			p = append(p, 0)
		}
	}
	return g.SetCurrentParticipationBits(p)
}

func (s *PremineGenesisConfig) setPrevEpochParticipation(g state.BeaconState) error {
	p, err := g.PreviousEpochParticipation()
	if err != nil {
		return err
	}
	missing := len(g.Validators()) - len(p)
	if missing > 0 {
		for range missing {
			p = append(p, 0)
		}
	}
	return g.SetPreviousParticipationBits(p)
}

func (s *PremineGenesisConfig) setSyncCommittees(g state.BeaconState) error {
	sc, err := altair.NextSyncCommittee(context.Background(), g)
	if err != nil {
		return err
	}
	if err = g.SetNextSyncCommittee(sc); err != nil {
		return err
	}
	return g.SetCurrentSyncCommittee(sc)
}

type rooter interface {
	HashTreeRoot() ([32]byte, error)
}

func (s *PremineGenesisConfig) setLatestBlockHeader(g state.BeaconState) error {
	var body rooter
	switch s.Version {
	case version.Zond:
		body = &qrysmpb.BeaconBlockBodyZond{
			RandaoReveal: make([]byte, fieldparams.MLDSA87SignatureLength),
			ExecutionData: &qrysmpb.ExecutionData{
				DepositRoot: make([]byte, 32),
				BlockHash:   make([]byte, 32),
			},
			Graffiti: make([]byte, 32),
			SyncAggregate: &qrysmpb.SyncAggregate{
				SyncCommitteeBits:       make([]byte, fieldparams.SyncCommitteeLength/8),
				SyncCommitteeSignatures: [][]byte{},
			},
			ExecutionPayload: &enginev1.ExecutionPayloadZond{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     make([]byte, 32),
				Transactions:  make([][]byte, 0),
				Withdrawals:   make([]*enginev1.Withdrawal, 0),
			},
		}
	default:
		return errUnsupportedVersion
	}

	root, err := body.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not hash tree root empty block body")
	}
	lbh := &qrysmpb.BeaconBlockHeader{
		ParentRoot: params.BeaconConfig().ZeroHash[:],
		StateRoot:  params.BeaconConfig().ZeroHash[:],
		BodyRoot:   root[:],
	}
	return g.SetLatestBlockHeader(lbh)
}

func (s *PremineGenesisConfig) setExecutionPayload(g state.BeaconState) error {
	gb := s.GB
	extraData := gb.Extra()
	if len(extraData) > 32 {
		extraData = extraData[:32]
	}

	var ed interfaces.ExecutionData
	switch s.Version {
	case version.Zond:
		payload := &enginev1.ExecutionPayloadZond{
			ParentHash:    gb.ParentHash().Bytes(),
			FeeRecipient:  gb.Coinbase().Bytes(),
			StateRoot:     gb.Root().Bytes(),
			ReceiptsRoot:  gb.ReceiptHash().Bytes(),
			LogsBloom:     gb.Bloom().Bytes(),
			PrevRandao:    params.BeaconConfig().ZeroHash[:],
			BlockNumber:   gb.NumberU64(),
			GasLimit:      gb.GasLimit(),
			GasUsed:       gb.GasUsed(),
			Timestamp:     gb.Time(),
			ExtraData:     extraData,
			BaseFeePerGas: bytesutil.PadTo(bytesutil.ReverseByteOrder(gb.BaseFee().Bytes()), fieldparams.RootLength),
			BlockHash:     gb.Hash().Bytes(),
			Transactions:  make([][]byte, 0),
			Withdrawals:   make([]*enginev1.Withdrawal, 0),
		}
		wep, err := blocks.WrappedExecutionPayloadZond(payload, 0)
		if err != nil {
			return err
		}
		eph, err := blocks.PayloadToHeaderZond(wep)
		if err != nil {
			return err
		}
		ed, err = blocks.WrappedExecutionPayloadHeaderZond(eph, 0)
		if err != nil {
			return err
		}
	default:
		return errUnsupportedVersion
	}
	return g.SetLatestExecutionPayloadHeader(ed)
}

func nZeroRoots(n uint64) [][]byte {
	roots := make([][]byte, n)
	zh := params.BeaconConfig().ZeroHash[:]
	for i := range n {
		roots[i] = zh
	}
	return roots
}

func nSetRoots(n uint64, r []byte) [][]byte {
	roots := make([][]byte, n)
	for i := range n {
		h := make([]byte, 32)
		copy(h, r)
		roots[i] = h
	}
	return roots
}
