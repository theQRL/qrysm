#!/bin/bash
qrvm=$GQRL_BIN      # "/home/martin/workspace/qrvm"
qrvmone=$QRVMO_BIN   #"/home/martin/workspace/qrvmone-statetest"

### Gqrl

if [[ -n "$qrvm" ]]; then
    echo "geth"
    cd ./cases
    # The traces
    for i in *.json; do
        $qrvm --json --nomemory --noreturndata statetest $i \
         2>../traces/$i.geth.stderr.txt \
         1>../traces/$i.geth.stdout.txt
    done
    # And the stateroots, where we invoke the qrvm the same way that
    # GetStateRoot does
    for i in *.json; do
        $qrvm statetest $i \
         2>../roots/$i.geth.stderr.txt \
         1>../roots/$i.geth.stdout.txt
    done
    cd ..
fi


### Nethermind

if [[ -n "$nethtest" ]]; then
    echo "nethermind"
    cd ./cases
    for i in *.json; do
        $nethtest --memory --trace --input $i \
         2>../traces/$i.nethermind.stderr.txt \
         1>../traces/$i.nethermind.stdout.txt
    done
    for i in *.json; do
        $nethtest --memory --neverTrace -s --input $i \
         2>../roots/$i.nethermind.stderr.txt \
         1>../roots/$i.nethermind.stdout.txt
    done
    cd ..
fi

# evmone
if [[ -n "$evmone" ]]; then
    echo "evmone"
    cd ./cases
    # The traces
    for i in *.json; do
        $evmone --trace $i \
         2>../traces/$i.evmone.stderr.txt
    done
    # And the stateroots, where we invoke the evm the same way that
    # GetStateRoot does
    for i in *.json; do
        $evmone --trace-summary $i \
         2>../roots/$i.evmone.stderr.txt
    done
    cd ..
fi

# retun
if [[ -n "$revm" ]]; then
    echo "revm"
    cd ./cases
    # The traces
    for i in *.json; do
        $revm statetest --json  $i \
         2>../traces/$i.revm.stderr.txt \
         1>../traces/$i.revm.stdout.txt
    done
    # And the stateroots, where we invoke the evm the same way that
    # GetStateRoot does
    for i in *.json; do
        $revm statetest --json-outcome $i \
         2>../roots/$i.revm.stderr.txt \
         1>../roots/$i.revm.stdout.txt
    done
    cd ..
fi

