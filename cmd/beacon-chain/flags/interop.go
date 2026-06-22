package flags

import (
	"github.com/urfave/cli/v2"
)

var (
	// InteropMockExecutionDataVotesFlag enables mocking the execution chain data put into blocks by proposers.
	InteropMockExecutionDataVotesFlag = &cli.BoolFlag{
		Name:  "interop-executiondata-votes",
		Usage: "Enable mocking of execution data votes for proposers to package into blocks",
	}

	// InteropGenesisTimeFlag specifies genesis time for state generation.
	InteropGenesisTimeFlag = &cli.Uint64Flag{
		Name: "interop-genesis-time",
		Usage: "Specify the genesis time for interop genesis state generation. Must be used with " +
			"--interop-num-validators",
	}
	// InteropNumValidatorsFlag specifies number of genesis validators for state generation.
	InteropNumValidatorsFlag = &cli.Uint64Flag{
		Name:  "interop-num-validators",
		Usage: "Specify number of genesis validators to generate for interop. Must be used with --interop-genesis-time",
	}
)
