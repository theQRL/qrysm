package types

import (
	"context"

	"github.com/google/uuid"
)

type WalletIDProvider interface {
	ID() uuid.UUID
}

type WalletNameProvider interface {
	Name() string
}

type WalletTypeProvider interface {
	Type() string
}

type WalletVersionProvider interface {
	Version() uint
}

type WalletLocker interface {
	Lock(ctx context.Context) error

	Unlock(ctx context.Context, passphrase []byte) error

	IsUnlocked(ctx context.Context) (bool, error)
}

type WalletAccountsProvider interface {
	Accounts(ctx context.Context) <-chan Account
}

type WalletAccountByIDProvider interface {
	AccountByID(ctx context.Context, id uuid.UUID) (Account, error)
}

type WalletAccountByNameProvider interface {
	AccountByName(ctx context.Context, name string) (Account, error)
}

type WalletAccountsByPathProvider interface {
	AccountsByPath(ctx context.Context, path string) <-chan Account
}

type WalletAccountCreator interface {
	CreateAccount(ctx context.Context, name string, passphrase []byte) (Account, error)
}

type WalletPathedAccountCreator interface {
	CreatePathedAccount(ctx context.Context, path string, name string, passphrase []byte) (Account, error)
}

type WalletDistributedAccountCreator interface {
	CreateDistributedAccount(ctx context.Context, name string, particpants uint32, signingThreshold uint32, passphrase []byte) (Account, error)
}

type WalletExporter interface {
	Export(ctx context.Context, passphrase []byte) ([]byte, error)
}

type WalletAccountImporter interface {
	ImportAccount(ctx context.Context, name string, key []byte, passphrase []byte) (Account, error)
}

type WalletDistributedAccountImporter interface {
	ImportDistributedAccount(ctx context.Context,
		name string,
		key []byte,
		signingThreshold uint32,
		verificationVector [][]byte,
		participants map[uint64]string,
		passphrase []byte) (Account, error)
}

type Wallet interface {
	WalletIDProvider
	WalletTypeProvider
	WalletNameProvider
	WalletVersionProvider
	WalletAccountsProvider
}
