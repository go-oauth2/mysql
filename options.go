package mysql

import (
	"time"

	"gopkg.in/gorp.v2"
)

type Option interface {
	apply(*Store)
}

type optionFunc func(store *Store)

func (f optionFunc) apply(store *Store) {
	f(store)
}

// WithTableName sets the table name for the store.
func WithTableName(tableName string) Option {
	return optionFunc(func(store *Store) {
		if tableName != "" {
			store.tableName = tableName
		}
	})
}

// WithSQLDialect sets the database for the store.
func WithSQLDialect(dialect gorp.MySQLDialect) Option {
	return optionFunc(func(store *Store) {
		store.db.Dialect = dialect
	})
}

// WithGCTimeInterval sets the time interval for garbage collection.
func WithGCTimeInterval(interval int) Option {
	return optionFunc(func(store *Store) {
		if interval != 0 {
			store.ticker = time.NewTicker(time.Second * time.Duration(interval))
		}
	})
}
