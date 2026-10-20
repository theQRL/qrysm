package iface

import (
	"errors"
)

var (
	// ErrExistingGenesisState is an error when the user attempts to save a different genesis state
	// when one already exists in a database.
	ErrExistingGenesisState = errors.New("genesis state exists already in the DB")
	// ErrEmbeddedGenesisMismatch is an error when the genesis block stored in the database was not
	// produced from the genesis state embedded in this build.
	ErrEmbeddedGenesisMismatch = errors.New("the genesis block in the DB does not match the embedded genesis state")
)
