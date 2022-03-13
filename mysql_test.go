package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-oauth2/oauth2/v4/models"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"gopkg.in/gorp.v2"
)

const (
	dsn = "root:@tcp(127.0.0.1:3306)/myapp_test?charset=utf8"
)

func TestTokenStore(t *testing.T) {
	Convey("Test mysql token store", t, func() {
		store := NewDefaultStore(NewConfig(dsn))
		defer store.clean()

		ctx := context.Background()

		Convey("Test authorization code store", func() {
			info := &models.Token{
				ClientID:      "1",
				UserID:        "1_1",
				RedirectURI:   "http://localhost/",
				Scope:         "all",
				Code:          "11_11_11",
				CodeCreateAt:  time.Now(),
				CodeExpiresIn: time.Second * 5,
			}
			err := store.Create(ctx, info)
			So(err, ShouldBeNil)

			cinfo, err := store.GetByCode(ctx, info.Code)
			So(err, ShouldBeNil)
			So(cinfo.GetUserID(), ShouldEqual, info.UserID)

			err = store.RemoveByCode(ctx, info.Code)
			So(err, ShouldBeNil)

			cinfo, err = store.GetByCode(ctx, info.Code)
			So(err, ShouldBeNil)
			So(cinfo, ShouldBeNil)
		})

		Convey("Test access token store", func() {
			info := &models.Token{
				ClientID:        "1",
				UserID:          "1_1",
				RedirectURI:     "http://localhost/",
				Scope:           "all",
				Access:          "1_1_1",
				AccessCreateAt:  time.Now(),
				AccessExpiresIn: time.Second * 5,
			}
			err := store.Create(ctx, info)
			So(err, ShouldBeNil)

			ainfo, err := store.GetByAccess(ctx, info.GetAccess())
			So(err, ShouldBeNil)
			So(ainfo.GetUserID(), ShouldEqual, info.GetUserID())

			err = store.RemoveByAccess(ctx, info.GetAccess())
			So(err, ShouldBeNil)

			ainfo, err = store.GetByAccess(ctx, info.GetAccess())
			So(err, ShouldBeNil)
			So(ainfo, ShouldBeNil)
		})

		Convey("Test refresh token store", func() {
			info := &models.Token{
				ClientID:         "1",
				UserID:           "1_2",
				RedirectURI:      "http://localhost/",
				Scope:            "all",
				Access:           "1_2_1",
				AccessCreateAt:   time.Now(),
				AccessExpiresIn:  time.Second * 5,
				Refresh:          "1_2_2",
				RefreshCreateAt:  time.Now(),
				RefreshExpiresIn: time.Second * 15,
			}
			err := store.Create(ctx, info)
			So(err, ShouldBeNil)

			ainfo, err := store.GetByAccess(ctx, info.GetAccess())
			So(err, ShouldBeNil)
			So(ainfo.GetUserID(), ShouldEqual, info.GetUserID())

			err = store.RemoveByAccess(ctx, info.GetAccess())
			So(err, ShouldBeNil)

			ainfo, err = store.GetByAccess(ctx, info.GetAccess())
			So(err, ShouldBeNil)
			So(ainfo, ShouldBeNil)

			rinfo, err := store.GetByRefresh(ctx, info.GetRefresh())
			So(err, ShouldBeNil)
			So(rinfo.GetUserID(), ShouldEqual, info.GetUserID())

			err = store.RemoveByRefresh(ctx, info.GetRefresh())
			So(err, ShouldBeNil)

			rinfo, err = store.GetByRefresh(ctx, info.GetRefresh())
			So(err, ShouldBeNil)
			So(rinfo, ShouldBeNil)
		})
	})
}

func TestNewStoreWithOpts_ShouldReturnStoreNotNil(t *testing.T) {
	// ARRANGE
	db, mockDB, _ := sqlmock.New()
	tableName := "custom_table_name"

	// Mock sql exec create table
	mockDB.ExpectExec(regexp.QuoteMeta("create table if not exists `custom_table_name` (`id` bigint not null primary key auto_increment, `expired_at` bigint, `code` varchar(255), `access` varchar(255), `refresh` varchar(255), `data` text) engine=InnoDB charset=UTF8;")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Mock query:
	mockDB.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM custom_table_name WHERE expired_at<=? OR (code='' AND access='' AND refresh='')")).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))

	// ACTION
	store := NewStoreWithOpts(db,
		WithTableName(tableName),
		WithSQLDialect(gorp.MySQLDialect{Engine: "InnoDB", Encoding: "UTF8"}),
		WithGCTimeInterval(1000),
	)

	defer store.clean()

	// ASSERT
	assert.NotNil(t, store)
	assert.NotNil(t, store.ticker)
	assert.Equal(t, store.tableName, tableName)
}
