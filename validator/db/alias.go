package db

import "github.com/theQRL/qrysm/validator/db/iface"

// Database defines the necessary methods for Qrysm's validator client backend which may be implemented by any
// key-value or relational database in practice. This is the full database interface which should
// not be used often. Prefer a more restrictive interface in this package.
type Database = iface.ValidatorDB
