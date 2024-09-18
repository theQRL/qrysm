package testing

import (
	"context"
	"testing"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/validator/db/iface"
	"github.com/theQRL/qrysm/validator/db/kv"
)

// SetupDB instantiates and returns a DB instance for the validator client.
func SetupDB(t testing.TB, pubkeys [][field_params.DilithiumPubkeyLength]byte) iface.ValidatorDB {
	db, err := kv.NewKVStore(context.Background(), t.TempDir(), &kv.Config{
		PubKeys: pubkeys,
	})
	if err != nil {
		t.Fatalf("Failed to instantiate DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Failed to close database: %v", err)
		}
		if err := db.ClearDB(); err != nil {
			t.Fatalf("Failed to clear database: %v", err)
		}
	})
	return db
}
