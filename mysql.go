package mysql

import (
	"database/sql"
	"os"
	"time"

	"gopkg.in/gorp.v2"
)

// StoreItem data item
type StoreItem struct {
	ID        int64  `db:"id,primarykey,autoincrement"`
	ExpiredAt int64  `db:"expired_at"`
	Code      string `db:"code,size:255"`
	Access    string `db:"access,size:255"`
	Refresh   string `db:"refresh,size:255"`
	Data      string `db:"data,size:2048"`
}

// NewConfig create mysql configuration instance
func NewConfig(dsn string) *Config {
	return &Config{
		DSN:          dsn,
		MaxLifetime:  time.Hour * 2,
		MaxOpenConns: 50,
		MaxIdleConns: 25,
	}
}

// Config mysql configuration
type Config struct {
	DSN          string
	MaxLifetime  time.Duration
	MaxOpenConns int
	MaxIdleConns int
}

// NewDefaultStore create mysql store instance
func NewDefaultStore(config *Config) *Store {
	return NewStore(config, "", 0)
}

// NewStore create mysql store instance,
// config mysql configuration,
// tableName table name (default oauth2_token),
// GC time interval (in seconds, default 600)
func NewStore(config *Config, tableName string, gcInterval int) *Store {
	db, err := sql.Open("mysql", config.DSN)
	if err != nil {
		panic(err)
	}

	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.MaxLifetime)

	return NewStoreWithDB(db, tableName, gcInterval)
}

// NewStoreWithDB create mysql store instance,
// db sql.DB,
// tableName table name (default oauth2_token),
// GC time interval (in seconds, default 600)
func NewStoreWithDB(db *sql.DB, tableName string, gcInterval int) *Store {
	// Init store with options
	store := NewStoreWithOpts(db,
		WithSQLDialect(gorp.MySQLDialect{Encoding: "UTF8", Engine: "MyISAM"}),
		WithTableName(tableName),
		WithGCTimeInterval(gcInterval),
	)

	go store.gc()
	return store
}

// NewStoreWithOpts create mysql store instance with apply custom input,
// db sql.DB,
// tableName table name (default oauth2_token),
// GC time interval (in seconds, default 600)
func NewStoreWithOpts(db *sql.DB, opts ...Option) *Store {
	// Init store with default value
	store := &Store{
		db:        &gorp.DbMap{Db: db, Dialect: gorp.MySQLDialect{Encoding: "UTF8", Engine: "MyISAM"}},
		tableName: "oauth2_token",
		stdout:    os.Stderr,
		ticker:    time.NewTicker(time.Second * time.Duration(600)),
	}

	// Apply with optional function
	for _, opt := range opts {
		opt.apply(store)
	}

	table := store.db.AddTableWithName(StoreItem{}, store.tableName)
	table.AddIndex("idx_code", "Btree", []string{"code"})
	table.AddIndex("idx_access", "Btree", []string{"access"})
	table.AddIndex("idx_refresh", "Btree", []string{"refresh"})
	table.AddIndex("idx_expired_at", "Btree", []string{"expired_at"})

	err := store.db.CreateTablesIfNotExists()
	if err != nil {
		panic(err)
	}

	_ = store.db.CreateIndex()

	go store.gc()
	return store
}
