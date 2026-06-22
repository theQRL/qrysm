package types

import "github.com/google/uuid"

type Store interface {
	Name() string

	StoreWallet(walletID uuid.UUID, walletName string, data []byte) error

	RetrieveWallets() <-chan []byte

	RetrieveWallet(walletName string) ([]byte, error)

	RetrieveWalletByID(walletID uuid.UUID) ([]byte, error)

	StoreAccount(walletID uuid.UUID, accountID uuid.UUID, data []byte) error

	RetrieveAccounts(walletID uuid.UUID) <-chan []byte

	RetrieveAccount(walletID uuid.UUID, accountID uuid.UUID) ([]byte, error)

	StoreAccountsIndex(walletID uuid.UUID, data []byte) error

	RetrieveAccountsIndex(walletID uuid.UUID) ([]byte, error)
}

type StoreProvider interface {
	Store() Store
}

type StoreLocationProvider interface {
	Location() string
}
