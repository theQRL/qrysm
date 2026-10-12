package slasher

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"go.opencensus.io/trace"
)

// Takes in a list of indexed attestation wrappers and returns any
// found attester slashings to the caller.
// checkSlashableAttestations returns the slashings detected among the given
// attestations and against the stored history. Slashings found before a
// failure to save the spans are returned together with the error: they are
// evidence in their own right (two signed, conflicting attestations), and the
// attestation records saved during the pass have by then replaced the older
// conflicting records, so a retry would no longer find them.
func (s *Service) checkSlashableAttestations(
	ctx context.Context, currentEpoch primitives.Epoch, atts []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing, error) {
	slashings := map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing{}

	log.Debug("Checking for double votes")
	start := time.Now()
	doubleVoteSlashings, err := s.checkDoubleVotes(ctx, atts)
	if err != nil {
		return nil, errors.Wrap(err, "could not check slashable double votes")
	}
	log.WithField("elapsed", time.Since(start)).Debug("Done checking double votes")
	for root, slashing := range doubleVoteSlashings {
		slashings[root] = slashing
	}

	// Save the attestation records to our database.
	// This must happen after the double-vote check so that the on-disk lookup
	// in checkDoubleVotes compares against previously saved attestations rather
	// than against the current batch (which would mask cross-batch double votes
	// because saves are keyed by validator+target epoch and would overwrite the
	// older record for the same key).
	if err := s.serviceCfg.Database.SaveAttestationRecordsForValidators(ctx, atts); err != nil {
		return nil, errors.Wrap(err, "could not save attestation records to DB")
	}

	groupedAtts := s.groupByValidatorChunkIndex(atts)
	log.WithField("numBatches", len(groupedAtts)).Debug("Batching attestations by validator chunk index")
	start = time.Now()
	batchTimes := make([]time.Duration, 0, len(groupedAtts))

	// With 256 validators and 16 epochs per chunk, each chunk holds 4096 uint16
	// elements (= 8 KiB). The flush bound is the store's per-transaction batch
	// size (10_000 chunks ≈ 80 MiB), so every flush is one transaction that
	// commits the chunks and their epoch markers together: a group that would
	// take the accumulation past the bound is flushed after the groups before
	// it, never with them.
	groupedAttsCount := len(groupedAtts)
	minChunkByChunkIndexByValidatorChunkIndex := make(map[uint64]map[uint64]Chunker, groupedAttsCount)
	maxChunkByChunkIndexByValidatorChunkIndex := make(map[uint64]map[uint64]Chunker, groupedAttsCount)
	// The latest epoch written for the validators of the accumulated groups.
	// The service's map is only advanced once a flush has committed, so an
	// error on any path out of here leaves memory claiming no progress the
	// disk does not hold.
	epochsToPersist := make(map[primitives.ValidatorIndex]primitives.Epoch)
	chunksAccumulated := 0
	flush := func() error {
		if err := s.saveProgressToDisk(ctx, minChunkByChunkIndexByValidatorChunkIndex, maxChunkByChunkIndexByValidatorChunkIndex, epochsToPersist); err != nil {
			return errors.Wrap(err, "could not save updated chunks to disk")
		}
		for idx, epoch := range epochsToPersist {
			s.latestEpochWrittenForValidator[idx] = epoch
		}
		minChunkByChunkIndexByValidatorChunkIndex = make(map[uint64]map[uint64]Chunker, groupedAttsCount)
		maxChunkByChunkIndexByValidatorChunkIndex = make(map[uint64]map[uint64]Chunker, groupedAttsCount)
		epochsToPersist = make(map[primitives.ValidatorIndex]primitives.Epoch)
		chunksAccumulated = 0
		return nil
	}

	for validatorChunkIdx, batch := range groupedAtts {
		innerStart := time.Now()
		attSlashings, minChunks, maxChunks, err := s.detectAllAttesterSlashings(ctx, &chunkUpdateArgs{
			validatorChunkIndex: validatorChunkIdx,
			currentEpoch:        currentEpoch,
		}, batch)
		if err != nil {
			return nil, err
		}
		for root, slashing := range attSlashings {
			slashings[root] = slashing
		}

		// Memoize updated chunks across iterations so many validator-chunk-index
		// passes are coalesced into a single disk save call, within the bound.
		groupChunks := len(minChunks) + len(maxChunks)
		if chunksAccumulated > 0 && chunksAccumulated+groupChunks > s.maxChunksPerFlush {
			if err := flush(); err != nil {
				return slashings, err
			}
		}
		minChunkByChunkIndexByValidatorChunkIndex[validatorChunkIdx] = minChunks
		maxChunkByChunkIndexByValidatorChunkIndex[validatorChunkIdx] = maxChunks
		chunksAccumulated += groupChunks
		for _, idx := range s.params.validatorIndicesInChunk(validatorChunkIdx) {
			epochsToPersist[idx] = currentEpoch
		}
		batchTimes = append(batchTimes, time.Since(innerStart))
	}

	// Flush the remaining groups, with their epoch markers.
	if err := flush(); err != nil {
		return slashings, err
	}
	var avgProcessingTimePerBatch time.Duration
	for _, dur := range batchTimes {
		avgProcessingTimePerBatch += dur
	}
	if avgProcessingTimePerBatch != time.Duration(0) {
		avgProcessingTimePerBatch = avgProcessingTimePerBatch / time.Duration(len(batchTimes))
	}
	log.WithFields(logrus.Fields{
		"numAttestations":                 len(atts),
		"numBatchesByValidatorChunkIndex": len(groupedAtts),
		"elapsed":                         time.Since(start),
		"avgBatchProcessingTime":          avgProcessingTimePerBatch,
	}).Info("Done checking slashable attestations")
	if len(slashings) > 0 {
		log.WithField("numSlashings", len(slashings)).Warn("Slashable attestation offenses found")
	}
	return slashings, nil
}

// Given a list of attestations all corresponding to a validator chunk index as well
// as the current epoch in time, we perform slashing detection.
// The process is as follows given a list of attestations:
//
//  1. Check for attester double votes using the list of attestations.
//  2. Group the attestations by chunk index.
//  3. Update the min and max spans for those grouped attestations, check if any slashings are
//     found in the process
//  4. Return the updated chunk maps so the caller can batch saves across validator-chunk-index
//     iterations (see checkSlashableAttestations).
//
// This function performs a lot of critical actions and is split into smaller helpers for cleanliness.
// It does NOT save the updated chunks itself — saving is deferred to the caller so multiple calls
// (one per validator chunk index) can be coalesced into a single bolt write transaction.
func (s *Service) detectAllAttesterSlashings(
	ctx context.Context,
	args *chunkUpdateArgs,
	attestations []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing, map[uint64]Chunker, map[uint64]Chunker, error) {
	// Separate chunk maps for min and max spans.
	updatedMinChunks := make(map[uint64]Chunker)
	updatedMaxChunks := make(map[uint64]Chunker)
	validatorIndices := s.params.validatorIndicesInChunk(args.validatorChunkIndex)

	minArgs := &chunkUpdateArgs{
		kind:                slashertypes.MinSpan,
		validatorChunkIndex: args.validatorChunkIndex,
		currentEpoch:        args.currentEpoch,
	}
	maxArgs := &chunkUpdateArgs{
		kind:                slashertypes.MaxSpan,
		validatorChunkIndex: args.validatorChunkIndex,
		currentEpoch:        args.currentEpoch,
	}

	// Update the min/max span chunks for the change of current epoch.
	for _, validatorIndex := range validatorIndices {
		if err := s.epochUpdateForValidator(ctx, minArgs, updatedMinChunks, validatorIndex); err != nil {
			return nil, nil, nil, errors.Wrapf(
				err,
				"could not update validator index for min chunks %d",
				validatorIndex,
			)
		}
		if err := s.epochUpdateForValidator(ctx, maxArgs, updatedMaxChunks, validatorIndex); err != nil {
			return nil, nil, nil, errors.Wrapf(
				err,
				"could not update validator index for max chunks %d",
				validatorIndex,
			)
		}
	}

	// Check for surrounding votes (MinSpan).
	//
	// The processing order within a batch decides which pairs of attestations
	// *in the same batch* can be detected: an attestation is only checked
	// against the spans as they stand when it is applied. In the min-span pass
	// an outer (surrounding) vote is found via min_spans[outer.source], which is
	// only populated once the inner vote (larger source) has been applied — so
	// inner votes must be applied first, i.e. descending source epoch. The
	// max-span pass is the mirror image: the inner vote is found via
	// max_spans[inner.source], populated by the outer vote (smaller source), so
	// ascending source epoch. Either pass alone then covers every surround pair
	// in the batch. Iterating a Go map here (the previous implementation grouped
	// by chunk index) made the order random per call, and the pair went
	// undetected whenever both passes happened to draw the wrong order.
	surroundingSlashings, err := s.updateSpans(ctx, updatedMinChunks, minArgs, sortBySourceEpoch(attestations, true))
	if err != nil {
		return nil, nil, nil, errors.Wrapf(
			err,
			"could not update min attestation spans for validator chunk index %d",
			args.validatorChunkIndex,
		)
	}

	// Check for surrounded votes (MaxSpan).
	surroundedSlashings, err := s.updateSpans(ctx, updatedMaxChunks, maxArgs, sortBySourceEpoch(attestations, false))
	if err != nil {
		return nil, nil, nil, errors.Wrapf(
			err,
			"could not update max attestation spans for validator chunk index %d",
			args.validatorChunkIndex,
		)
	}

	slashings := make(map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing, len(surroundingSlashings)+len(surroundedSlashings))
	for root, slashing := range surroundingSlashings {
		slashings[root] = slashing
	}
	for root, slashing := range surroundedSlashings {
		slashings[root] = slashing
	}

	return slashings, updatedMinChunks, updatedMaxChunks, nil
}

// Check for attester slashing double votes by looking at every single validator index
// in each attestation's attesting indices and checking if there already exist records for such
// attestation's target epoch. If so, we append a double vote slashing object to a list of slashings
// we return to the caller.
func (s *Service) checkDoubleVotes(
	ctx context.Context, attestations []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.checkDoubleVotes")
	defer span.End()
	// We check if there are any slashable double votes in the input list
	// of attestations with respect to each other.
	slashings := map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing{}
	existingAtts := make(map[string]*slashertypes.IndexedAttestationWrapper)
	for _, att := range attestations {
		for _, valIdx := range att.IndexedAttestation.AttestingIndices {
			key := uintToString(uint64(att.IndexedAttestation.Data.Target.Epoch)) + ":" + uintToString(valIdx)
			existingAtt, ok := existingAtts[key]
			if !ok {
				existingAtts[key] = att
				continue
			}
			if att.SigningRoot != existingAtt.SigningRoot {
				doubleVotesTotal.Inc()
				slashing := &qrysmpb.AttesterSlashing{
					Attestation_1: existingAtt.IndexedAttestation,
					Attestation_2: att.IndexedAttestation,
				}
				root, err := slashing.HashTreeRoot()
				if err != nil {
					return nil, errors.Wrap(err, "could not hash tree root for attester slashing")
				}
				slashings[root] = slashing
			}
		}
	}

	// We check if there are any slashable double votes in the input list
	// of attestations with respect to our database.
	moreSlashings, err := s.checkDoubleVotesOnDisk(ctx, attestations)
	if err != nil {
		return nil, errors.Wrap(err, "could not check attestation double votes on disk")
	}
	for root, slashing := range moreSlashings {
		slashings[root] = slashing
	}
	return slashings, nil
}

// Check for double votes in our database given a list of incoming attestations.
func (s *Service) checkDoubleVotesOnDisk(
	ctx context.Context, attestations []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.checkDoubleVotesOnDisk")
	defer span.End()
	doubleVotes, err := s.serviceCfg.Database.CheckAttesterDoubleVotes(
		ctx, attestations,
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve potential double votes from disk")
	}
	doubleVoteSlashings := map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing{}
	for _, doubleVote := range doubleVotes {
		doubleVotesTotal.Inc()
		slashing := &qrysmpb.AttesterSlashing{
			Attestation_1: doubleVote.PrevAttestationWrapper.IndexedAttestation,
			Attestation_2: doubleVote.AttestationWrapper.IndexedAttestation,
		}
		root, err := slashing.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not hash tree root for attester slashing")
		}
		doubleVoteSlashings[root] = slashing
	}
	return doubleVoteSlashings, nil
}

// This function updates the slashing spans for a given validator for a change in epoch
// since the last epoch we have recorded for the validator. For example, if the last epoch a validator
// has written is N, and the current epoch is N+5, we update entries in the slashing spans
// with their neutral element for epochs N+1 to N+4. This also puts any loaded chunks in a
// map used as a cache for further processing and minimizing database reads later on.
func (s *Service) epochUpdateForValidator(
	ctx context.Context,
	args *chunkUpdateArgs,
	updatedChunks map[uint64]Chunker,
	validatorIndex primitives.ValidatorIndex,
) error {
	latestWritten, ok := s.latestEpochWrittenForValidator[validatorIndex]
	if !ok {
		return nil
	}
	// Start *after* the latest written epoch. The cell at `latestWritten`
	// already contains real span data from the previous detection pass —
	// overwriting it with the neutral element here would erase surround-vote
	// information until the circular buffer wraps. (upstream PR #13620
	// off-by-one fix.)
	epoch, err := latestWritten.SafeAdd(1)
	if err != nil {
		return errors.Wrap(err, "could not add 1 to latest epoch written")
	}
	// Bound the loop to historyLength iterations: the min/max span chunks are
	// a circular buffer of length `historyLength`, so writing the neutral
	// element more than `historyLength` epochs back is wasted work — any
	// such cell would be overwritten by a later iteration as the buffer
	// wraps. Without this bound, a slasher cold-starting after a long
	// downtime would loop O(currentEpoch - lastWritten) times per validator,
	// turning startup into many minutes of redundant work on a long-lived
	// chain. (upstream PR #13620)
	if epoch <= args.currentEpoch && args.currentEpoch-epoch >= s.params.historyLength {
		epoch = args.currentEpoch + 1 - s.params.historyLength
	}
	for epoch <= args.currentEpoch {
		chunkIdx := s.params.chunkIndex(epoch)
		currentChunk, err := s.getChunk(ctx, args, updatedChunks, chunkIdx)
		if err != nil {
			return err
		}
		for s.params.chunkIndex(epoch) == chunkIdx && epoch <= args.currentEpoch {
			if err := setChunkRawDistance(
				s.params,
				currentChunk.Chunk(),
				validatorIndex,
				epoch,
				currentChunk.NeutralElement(),
			); err != nil {
				return err
			}
			updatedChunks[chunkIdx] = currentChunk
			epoch++
		}
	}
	return nil
}

// Updates spans and detects any slashable attester offenses along the way.
//  1. Determine the chunks we need to use for updating for the validator indices
//     in a validator chunk index, then retrieve those chunks from the database.
//  2. Using the chunks from step (1):
//     for every attestation, in the order given (see detectAllAttesterSlashings
//     for why the order matters):
//     for each validator in the attestation's attesting indices:
//     - Check if the attestation is slashable, if so return a slashing object.
//  3. Save the updated chunks to disk.
func (s *Service) updateSpans(
	ctx context.Context,
	updatedChunks map[uint64]Chunker,
	args *chunkUpdateArgs,
	attestations []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.updateSpans")
	defer span.End()

	// Apply the attestations to the related chunks and find any
	// slashings along the way.
	slashings := map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing{}
	for _, att := range attestations {
		for _, validatorIdx := range att.IndexedAttestation.AttestingIndices {
			validatorIndex := primitives.ValidatorIndex(validatorIdx)
			computedValidatorChunkIdx := s.params.validatorChunkIndex(validatorIndex)

			// Every validator chunk index represents a range of validators.
			// If it possible that the validator index in this loop iteration is
			// not part of the validator chunk index we are updating chunks for.
			//
			// For example, if there are 4 validators per validator chunk index,
			// then validator chunk index 0 contains validator indices [0, 1, 2, 3]
			// If we see an attestation with attesting indices [3, 4, 5] and we are updating
			// chunks for validator chunk index 0, only validator index 3 should make
			// it past this line.
			if args.validatorChunkIndex != computedValidatorChunkIdx {
				continue
			}
			slashing, err := s.applyAttestationForValidator(
				ctx,
				args,
				validatorIndex,
				updatedChunks,
				att,
			)
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"could not apply attestation for validator index %d",
					validatorIndex,
				)
			}
			if slashing == nil {
				continue
			}
			root, err := slashing.HashTreeRoot()
			if err != nil {
				return nil, errors.Wrap(err, "could not hash tree root for attester slashing")
			}
			slashings[root] = slashing
		}
	}

	// Write the updated chunks to disk.
	return slashings, nil
}

// Checks if an incoming attestation is slashable based on the validator chunk it
// corresponds to. If a slashable offense is found, we return it to the caller.
// If not, then update every single chunk the attestation covers, starting from its
// source epoch up to its target.
func (s *Service) applyAttestationForValidator(
	ctx context.Context,
	args *chunkUpdateArgs,
	validatorIndex primitives.ValidatorIndex,
	chunksByChunkIdx map[uint64]Chunker,
	attestation *slashertypes.IndexedAttestationWrapper,
) (*qrysmpb.AttesterSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.applyAttestationForValidator")
	defer span.End()
	sourceEpoch := attestation.IndexedAttestation.Data.Source.Epoch
	targetEpoch := attestation.IndexedAttestation.Data.Target.Epoch

	attestationDistance.Observe(float64(targetEpoch) - float64(sourceEpoch))

	chunkIdx := s.params.chunkIndex(sourceEpoch)
	chunk, err := s.getChunk(ctx, args, chunksByChunkIdx, chunkIdx)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get chunk at index %d", chunkIdx)
	}

	// Check slashable, if so, return the slashing.
	slashing, err := chunk.CheckSlashable(
		ctx,
		s.serviceCfg.Database,
		validatorIndex,
		attestation,
	)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"could not check if attestation for validator index %d is slashable",
			validatorIndex,
		)
	}
	if slashing != nil {
		return slashing, nil
	}

	// Get the first start epoch for the chunk. If it does not exist or
	// is not possible based on the input arguments, do not continue with the update.
	startEpoch, exists := chunk.StartEpoch(sourceEpoch, args.currentEpoch)
	if !exists {
		return nil, nil
	}

	// Given a single attestation could span across multiple chunks
	// for a validator min or max span, we attempt to update the current chunk
	// for the source epoch of the attestation. If the update function tells
	// us we need to proceed to the next chunk, we continue by determining
	// the start epoch of the next chunk. We exit once no longer need to
	// keep updating chunks.
	for {
		chunkIdx = s.params.chunkIndex(startEpoch)
		chunk, err := s.getChunk(ctx, args, chunksByChunkIdx, chunkIdx)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get chunk at index %d", chunkIdx)
		}
		keepGoing, err := chunk.Update(
			&chunkUpdateArgs{
				chunkIndex:   chunkIdx,
				currentEpoch: args.currentEpoch,
			},
			validatorIndex,
			startEpoch,
			targetEpoch,
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"could not update chunk at chunk index %d for validator index %d and current epoch %d",
				chunkIdx,
				validatorIndex,
				args.currentEpoch,
			)
		}
		// We update the chunksByChunkIdx map with the chunk we just updated.
		chunksByChunkIdx[chunkIdx] = chunk
		if !keepGoing {
			break
		}
		// Move to first epoch of next chunk if needed.
		startEpoch = chunk.NextChunkStartEpoch(startEpoch)
	}
	return nil, nil
}

// Retrieves a chunk at a chunk index from a map. If such chunk does not exist, which
// should be rare (occurring when we receive an attestation with source and target epochs
// that span multiple chunk indices), then we fallback to fetching from disk.
func (s *Service) getChunk(
	ctx context.Context,
	args *chunkUpdateArgs,
	chunksByChunkIdx map[uint64]Chunker,
	chunkIdx uint64,
) (Chunker, error) {
	chunk, ok := chunksByChunkIdx[chunkIdx]
	if ok {
		return chunk, nil
	}
	// We can ensure we load the appropriate chunk we need by fetching from the DB.
	diskChunks, err := s.loadChunks(ctx, args, []uint64{chunkIdx})
	if err != nil {
		return nil, errors.Wrapf(err, "could not load chunk at index %d", chunkIdx)
	}
	if chunk, ok := diskChunks[chunkIdx]; ok {
		return chunk, nil
	}
	return nil, fmt.Errorf("could not retrieve chunk at chunk index %d from disk", chunkIdx)
}

// Load chunks for a specified list of chunk indices. We attempt to load it from the database.
// If the data exists, then we initialize a chunk of a specified kind. Otherwise, we create
// an empty chunk, add it to our map, and then return it to the caller.
func (s *Service) loadChunks(
	ctx context.Context,
	args *chunkUpdateArgs,
	chunkIndices []uint64,
) (map[uint64]Chunker, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.loadChunks")
	defer span.End()
	chunkKeys := make([][]byte, 0, len(chunkIndices))
	for _, chunkIdx := range chunkIndices {
		chunkKeys = append(chunkKeys, s.params.flatSliceID(args.validatorChunkIndex, chunkIdx))
	}
	rawChunks, chunksExist, err := s.serviceCfg.Database.LoadSlasherChunks(ctx, args.kind, chunkKeys)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"could not load slasher chunk index",
		)
	}
	chunksByChunkIdx := make(map[uint64]Chunker, len(rawChunks))
	for i := range rawChunks {
		// If the chunk exists in the database, we initialize it from the raw bytes data.
		// If it does not exist, we initialize an empty chunk.
		var chunk Chunker
		switch args.kind {
		case slashertypes.MinSpan:
			if chunksExist[i] {
				chunk, err = MinChunkSpansSliceFrom(s.params, rawChunks[i])
			} else {
				chunk = EmptyMinSpanChunksSlice(s.params)
			}
		case slashertypes.MaxSpan:
			if chunksExist[i] {
				chunk, err = MaxChunkSpansSliceFrom(s.params, rawChunks[i])
			} else {
				chunk = EmptyMaxSpanChunksSlice(s.params)
			}
		}
		if err != nil {
			return nil, errors.Wrap(err, "could not initialize chunk")
		}
		chunksByChunkIdx[chunkIndices[i]] = chunk
	}
	return chunksByChunkIdx, nil
}

// Saves updated chunks to disk given the required database schema. Used by tests and
// for the older one-validator-chunk-index-at-a-time save path. Production callers should
// prefer saveProgressToDisk which coalesces chunks across many validator-chunk-index
// iterations into a single bolt transaction (capped by the DB-layer's batchSize).
func (s *Service) saveUpdatedChunks(
	ctx context.Context,
	args *chunkUpdateArgs,
	updatedChunksByChunkIdx map[uint64]Chunker,
) error {
	ctx, span := trace.StartSpan(ctx, "Slasher.saveUpdatedChunks")
	defer span.End()
	chunkKeys := make([][]byte, 0, len(updatedChunksByChunkIdx))
	chunks := make([][]uint16, 0, len(updatedChunksByChunkIdx))
	for chunkIdx, chunk := range updatedChunksByChunkIdx {
		chunkKeys = append(chunkKeys, s.params.flatSliceID(args.validatorChunkIndex, chunkIdx))
		chunks = append(chunks, chunk.Chunk())
	}
	chunksSavedTotal.Add(float64(len(chunks)))
	return s.serviceCfg.Database.SaveSlasherChunks(ctx, args.kind, chunkKeys, chunks)
}

// saveProgressToDisk flushes the accumulated min and max span chunks
// (validatorChunkIndex → chunkIndex → Chunker) and the latest epoch written for
// the validators they cover in one save call, which writes the epochs in the
// same transaction as the chunks. The epoch markers used to be flushed once per
// epoch: a validator whose chunk was first processed later in an epoch had its
// chunks on disk ahead of its marker, and the next start reset those spans to
// the neutral element, so a persisted vote no longer detected the vote that
// surrounded it. Combined with the deferred-flush logic in
// checkSlashableAttestations, this collapses what was previously one save call
// per validator-chunk-index into one save call per maxChunkBeforeFlush worth of
// work, which keeps the slasher receive queue from backing up under sustained
// load.
func (s *Service) saveProgressToDisk(
	ctx context.Context,
	minChunkByChunkIndexByValidatorChunkIndex map[uint64]map[uint64]Chunker,
	maxChunkByChunkIndexByValidatorChunkIndex map[uint64]map[uint64]Chunker,
	epochByValidator map[primitives.ValidatorIndex]primitives.Epoch,
) error {
	ctx, span := trace.StartSpan(ctx, "Slasher.saveProgressToDisk")
	defer span.End()

	minKeys, minChunks := s.flattenChunks(minChunkByChunkIndexByValidatorChunkIndex)
	maxKeys, maxChunks := s.flattenChunks(maxChunkByChunkIndexByValidatorChunkIndex)
	if len(minKeys)+len(maxKeys) == 0 && len(epochByValidator) == 0 {
		return nil
	}
	if err := s.serviceCfg.Database.SaveSlasherChunksAndEpochs(ctx, minKeys, minChunks, maxKeys, maxChunks, epochByValidator); err != nil {
		return err
	}
	chunksSavedTotal.Add(float64(len(minKeys) + len(maxKeys)))
	return nil
}

// flattenChunks turns a nested chunk map into parallel key and chunk slices.
func (s *Service) flattenChunks(chunkByChunkIndexByValidatorChunkIndex map[uint64]map[uint64]Chunker) ([][]byte, [][]uint16) {
	chunksCount := 0
	for _, chunkByChunkIndex := range chunkByChunkIndexByValidatorChunkIndex {
		chunksCount += len(chunkByChunkIndex)
	}
	chunkKeys := make([][]byte, 0, chunksCount)
	chunks := make([][]uint16, 0, chunksCount)
	for validatorChunkIndex, chunkByChunkIndex := range chunkByChunkIndexByValidatorChunkIndex {
		for chunkIndex, chunk := range chunkByChunkIndex {
			chunkKeys = append(chunkKeys, s.params.flatSliceID(validatorChunkIndex, chunkIndex))
			chunks = append(chunks, chunk.Chunk())
		}
	}
	return chunkKeys, chunks
}

// maxAttestationRetries bounds how often an attestation whose spans could not
// be saved is queued again for the next tick.
const maxAttestationRetries = 8

// requeueAttestations puts the attestations of a pass whose spans were not
// saved back on the queue for the next tick, up to maxAttestationRetries
// times each; beyond that they are dropped, with their span updates.
func (s *Service) requeueAttestations(atts []*slashertypes.IndexedAttestationWrapper) {
	if s.attestationRetries == nil {
		s.attestationRetries = make(map[[32]byte]uint8)
	}
	requeued := make([]*slashertypes.IndexedAttestationWrapper, 0, len(atts))
	for _, att := range atts {
		n := s.attestationRetries[att.SigningRoot]
		if n >= maxAttestationRetries {
			delete(s.attestationRetries, att.SigningRoot)
			log.WithField("signingRoot", fmt.Sprintf("%#x", att.SigningRoot)).
				Error("Dropping an attestation whose spans could not be saved after repeated attempts")
			continue
		}
		s.attestationRetries[att.SigningRoot] = n + 1
		requeued = append(requeued, att)
	}
	s.attsQueue.extend(requeued)
}

// forgetAttestationRetries clears the retry counts of attestations whose
// spans have been saved.
func (s *Service) forgetAttestationRetries(atts []*slashertypes.IndexedAttestationWrapper) {
	if len(s.attestationRetries) == 0 {
		return
	}
	for _, att := range atts {
		delete(s.attestationRetries, att.SigningRoot)
	}
}
