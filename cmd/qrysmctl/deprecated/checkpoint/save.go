package checkpoint

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var saveCmd = &cli.Command{
	Name:   "save",
	Usage:  "deprecated - please use 'qrysmctl checkpoint-sync download' instead!",
	Action: cliActionDeprecatedSave,
}

func cliActionDeprecatedSave(_ *cli.Context) error {
	return fmt.Errorf("This command has moved. Please use 'qrysmctl checkpoint-sync download' instead!")
}
