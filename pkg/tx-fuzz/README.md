# tx-fuzz

tx-fuzz is a package containing helpful functions to create random transactions. 
It can be used to easily access fuzzed transactions from within other programs.

## Usage

```
cd cmd/livefuzzer
go build
```

Run a Gqrl execution layer client locally in a standalone bash window.
Tx-fuzz sends transactions to port `8545` by default.

```
gqrl --http --http.port 8545
```

Run livefuzzer.

```
./livefuzzer spam
```