package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	v1alpha1 "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type votes struct {
	l sync.RWMutex

	hashes     map[[32]byte]uint
	roots      map[[32]byte]uint
	counts     map[uint64]uint
	votes      map[[32]byte]*v1alpha1.ExecutionData
	voteCounts map[[32]byte]uint
	total      uint
}

func newVotes() *votes {
	return &votes{
		hashes:     make(map[[32]byte]uint),
		roots:      make(map[[32]byte]uint),
		counts:     make(map[uint64]uint),
		votes:      make(map[[32]byte]*v1alpha1.ExecutionData),
		voteCounts: make(map[[32]byte]uint),
	}
}

func (v *votes) Insert(blk interfaces.ReadOnlyBeaconBlock) {
	v.l.Lock()
	defer v.l.Unlock()

	e1d := blk.Body().ExecutionData()
	htr, err := e1d.HashTreeRoot()
	if err != nil {
		panic(err)
	}
	v.hashes[bytesutil.ToBytes32(e1d.BlockHash)]++
	v.roots[bytesutil.ToBytes32(e1d.DepositRoot)]++
	v.counts[e1d.DepositCount]++
	v.votes[htr] = e1d
	v.voteCounts[htr]++
	v.total++
}

func (v *votes) Report() string {
	v.l.RLock()
	defer v.l.RUnlock()
	format := `====ExecutionData Voting Report====

Total votes: %d

Block Hashes
%s
Deposit Roots
%s
Deposit Counts
%s
Votes
%s
`
	var blockHashes strings.Builder
	for r, cnt := range v.hashes {
		_, _ = fmt.Fprintf(&blockHashes, "%#x=%d\n", r, cnt)
	}
	var depositRoots strings.Builder
	for r, cnt := range v.roots {
		_, _ = fmt.Fprintf(&depositRoots, "%#x=%d\n", r, cnt)
	}
	var depositCounts strings.Builder
	for dc, cnt := range v.counts {
		_, _ = fmt.Fprintf(&depositCounts, "%d=%d\n", dc, cnt)
	}
	var votes strings.Builder
	for htr, e1d := range v.votes {
		_, _ = fmt.Fprintf(&votes, "%s=%d\n", e1d.String(), v.voteCounts[htr])
	}

	return fmt.Sprintf(
		format,
		v.total,
		blockHashes.String(),
		depositRoots.String(),
		depositCounts.String(),
		votes.String(),
	)
}
