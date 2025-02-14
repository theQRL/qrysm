## Qcli (Qrysm CLI)

This is a utility to help users perform Ethereum consensus specific commands.

### Usage

*Name:*  
   **qcli** - A command line utility to run Ethereum consensus specific commands

*Usage:*  
   qcli [global options] command [command options] [arguments...]

*Commands:*
     help, h  Shows a list of commands or help for one command
   state-transition:
     state-transition  Subcommand to run manual state transitions


*Flags:*  
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)

*State Transition Subcommand:*
   qcli state-transition - Subcommand to run manual state transitions

*State Transition Usage:*:
   qcli state-transition [command options] [arguments...]


*State Transition Flags:*
   --block-path value              Path to block file(ssz)
   --pre-state-patch value           Path to pre state file(ssz)
   --expected-post-state-path value  Path to expected post state file(ssz)
   --help, -h                     show help (default: false)



### Example

To use qcli manual state transition:

```
bazel run //tools/qcli:qcli -- state-transition --block-path /path/to/block.ssz --pre-state-path /path/to/state.ssz
```

