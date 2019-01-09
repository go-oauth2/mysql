package mysql

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/json-iterator/go"
	"gopkg.in/gorp.v2"
	"gopkg.in/oauth2.v3"
	"gopkg.in/oauth2.v3/models"
)

// StoreItem data item
type StoreItem struct {
	ID        int64  `db:"id,primarykey,autoincrement"`
	ExpiredAt int64  `db:"expired_at"`
	Code      string `db:"code,size:512"`
	Access    string `db:"access,size:512"`
	Refresh   string `db:"refresh,size:512"`
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
	store := &Store{
		db:        &gorp.DbMap{Db: db, Dialect: gorp.MySQLDialect{Encoding: "UTF8", Engine: "MyISAM"}},
		tableName: "oauth2_token",
		stdout:    os.Stderr,
	}
	if tableName != "" {
		store.tableName = tableName
	}

	interval := 600
	if gcInterval > 0 {
		interval = gcInterval
	}
	store.ticker = time.NewTicker(time.Second * time.Duration(interval))

	table := store.db.AddTableWithName(StoreItem{}, store.tableName)
	table.AddIndex("idx_code", "Btree", []string{"code"})
	table.AddIndex("idx_access", "Btree", []string{"access"})
	table.AddIndex("idx_refresh", "Btree", []string{"refresh"})
	table.AddIndex("idx_expired_at", "Btree", []string{"expired_at"})

	err := store.db.CreateTablesIfNotExists()
	if err != nil {
		panic(err)
	}
	store.db.CreateIndex()

	go store.gc()
	return store
}

// Store mysql token store
type Store struct {
	tableName string
	db        *gorp.DbMap
	stdout    io.Writer
	ticker    *time.Ticker
}

// SetStdout set error output
func (s *Store) SetStdout(stdout io.Writer) *Store {
	s.stdout = stdout
	return s
}

// Close close the store
func (s *Store) Close() {
	s.ticker.Stop()
	s.db.Db.Close()
}

func (s *Store) gc() {
	for range s.ticker.C {
		s.clean()
	}
}

func (s *Store) clean() {
	now := time.Now().Unix()
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE expired_at<=? OR (code='' AND access='' AND refresh='')", s.tableName)
	n, err := s.db.SelectInt(query, now)
	if err != nil || n == 0 {
		if err != nil {
			s.errorf(err.Error())
		}
		return
	}

	_, err = s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE expired_at<=? OR (code='' AND access='' AND refresh='')", s.tableName), now)
	if err != nil {
		s.errorf(err.Error())
	}
}

func (s *Store) errorf(format string, args ...interface{}) {
	if s.stdout != nil {
		buf := fmt.Sprintf("[OAUTH2-MYSQL-ERROR]: "+format, args...)
		s.stdout.Write([]byte(buf))
	}
}

// Create create and store the new token information
func (s *Store) Create(info oauth2.TokenInfo) error {
	buf, _ := jsoniter.Marshal(info)
	item := &StoreItem{
		Data: string(buf),
	}

	if code := info.GetCode(); code != "" {
		item.Code = code
		item.ExpiredAt = info.GetCodeCreateAt().Add(info.GetCodeExpiresIn()).Unix()
	} else {
		item.Access = info.GetAccess()
		item.ExpiredAt = info.GetAccessCreateAt().Add(info.GetAccessExpiresIn()).Unix()

		if refresh := info.GetRefresh(); refresh != "" {
			item.Refresh = info.GetRefresh()
			item.ExpiredAt = info.GetRefreshCreateAt().Add(info.GetRefreshExpiresIn()).Unix()
		}
	}

	return s.db.Insert(item)
}

// RemoveByCode delete the authorization code
func (s *Store) RemoveByCode(code string) error {
	query := fmt.Sprintf("UPDATE %s SET code='' WHERE code=? LIMIT 1", s.tableName)
	_, err := s.db.Exec(query, code)
	if err != nil && err == sql.ErrNoRows {
		return nil
	}
	return err
}

// RemoveByAccess use the access token to delete the token information
func (s *Store) RemoveByAccess(access string) error {
	query := fmt.Sprintf("UPDATE %s SET access='' WHERE access=? LIMIT 1", s.tableName)
	_, err := s.db.Exec(query, access)
	if err != nil && err == sql.ErrNoRows {
		return nil
	}
	return err
}

// RemoveByRefresh use the refresh token to delete the token information
func (s *Store) RemoveByRefresh(refresh string) error {
	query := fmt.Sprintf("UPDATE %s SET refresh='' WHERE refresh=? LIMIT 1", s.tableName)
	_, err := s.db.Exec(query, refresh)
	if err != nil && err == sql.ErrNoRows {
		return nil
	}
	return err
}

func (s *Store) toTokenInfo(data string) oauth2.TokenInfo {
	var tm models.Token
	jsoniter.Unmarshal([]byte(data), &tm)
	return &tm
}

// GetByCode use the authorization code for token information data
func (s *Store) GetByCode(code string) (oauth2.TokenInfo, error) {
	if code == "" {
		return nil, nil
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE code=? LIMIT 1", s.tableName)
	var item StoreItem
	err := s.db.SelectOne(&item, query, code)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s.toTokenInfo(item.Data), nil
}

// GetByAccess use the access token for token information data
func (s *Store) GetByAccess(access string) (oauth2.TokenInfo, error) {
	if access == "" {
		return nil, nil
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE access=? LIMIT 1", s.tableName)
	var item StoreItem
	err := s.db.SelectOne(&item, query, access)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s.toTokenInfo(item.Data), nil
}

// GetByRefresh use the refresh token for token information data
func (s *Store) GetByRefresh(refresh string) (oauth2.TokenInfo, error) {
	if refresh == "" {
		return nil, nil
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE refresh=? LIMIT 1", s.tableName)
	var item StoreItem
	err := s.db.SelectOne(&item, query, refresh)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s.toTokenInfo(item.Data), nil
}
