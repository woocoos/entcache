package entcache

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/suite"
	"github.com/tsingsun/woocoo/pkg/cache"
	"github.com/tsingsun/woocoo/pkg/cache/redisc"
	"github.com/tsingsun/woocoo/pkg/conf"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var _ cache.Cache = (*mockCache)(nil)

type mockCache struct {
}

func (m mockCache) Get(ctx context.Context, key string, value any, opts ...cache.Option) error {
	//TODO implement me
	panic("implement me")
}

func (m mockCache) Set(ctx context.Context, key string, value any, opts ...cache.Option) error {
	//TODO implement me
	panic("implement me")
}

func (m mockCache) Has(ctx context.Context, key string) bool {
	//TODO implement me
	panic("implement me")
}

func (m mockCache) Del(ctx context.Context, key string) error {
	//TODO implement me
	panic("implement me")
}

func (m mockCache) IsNotFound(err error) bool {
	//TODO implement me
	panic("implement me")
}

type driverSuite struct {
	suite.Suite
	DB    *sql.Driver
	Redis *miniredis.Miniredis
}

func TestDriverSuite(t *testing.T) {
	suite.Run(t, new(driverSuite))
}

func (t *driverSuite) SetupSuite() {
	db, err := sql.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	t.Require().NoError(err)
	t.DB = db
	t.Require().NoError(t.DB.Exec(context.Background(),
		"create table users (id integer primary key autoincrement, age float)", []any{}, nil))
	t.Require().NoError(t.DB.Exec(context.Background(),
		"insert into users values (?,?)", []any{1, 20.1}, nil))
	t.Redis, err = miniredis.Run()
	t.Require().NoError(err)
}

func (t *driverSuite) TestDriver() {
	query := func(drv *Driver) {
		rows := &sql.Rows{}
		err := drv.Query(context.Background(), "SELECT age FROM users", []any{20.1, 30.2, 40.5}, rows)
		t.Require().NoError(err)
		defer rows.Close()
	}
	t.Run("default", func() {
		drv := NewDriver(t.DB)
		query(drv)
	})
	t.Run("withTTL", func() {
		drv := NewDriver(t.DB, WithConfiguration(conf.NewFromStringMap(map[string]any{
			"hashQueryTTL": time.Second,
			"name":         "withTTL",
		})))
		query(drv)
		query(drv)
		time.Sleep(time.Second)
		query(drv)
		t.Equal(3, int(drv.stats.Gets))
		t.Equal(1, int(drv.stats.Hits))
	})
	t.Run("redis", func() {
		cnfstr := `
driverName: drvierTest
storeKey: drvierTest
`
		cnf := conf.NewFromBytes([]byte(cnfstr))
		cnf.Parser().Set("addrs", []string{t.Redis.Addr()})
		_, err := redisc.New(cnf)
		t.Require().NoError(err)
		drv := NewDriver(t.DB, WithConfiguration(cnf))
		query(drv)
	})
	t.Run("with cache", func() {
		drv := NewDriver(t.DB, WithCache(mockCache{}))
		t.Panics(func() {
			query(drv)
		})
	})
}

func (t *driverSuite) TestWithXXEntryKey() {
	var dest struct {
		id  int
		age float64
	}

	query := func(drv *Driver, ctx context.Context, query string, args any) {
		rows := &sql.Rows{}
		err := drv.Query(ctx, query, args, rows)
		t.Require().NoError(err)
		rows.Next()
		_ = rows.Scan(&dest.id, &dest.age)
		_ = rows.Close()
	}
	t.Run("fieldQuery", func() {
		drv := NewDriver(t.DB, WithConfiguration(conf.NewFromStringMap(map[string]any{
			"hashQueryTTL": time.Second,
			"name":         "fieldQuery",
			"cachePrefix":  "fieldQuery:",
		})))
		all := "SELECT * FROM users where id=?"
		query(drv, WithEntryKey(context.Background(), "User", 1), all, []any{1})
		query(drv, WithRefEntryKey(context.Background(), "User", 1), "SELECT age FROM users where id=?", []any{1})

		t.Equal(uint64(2), drv.stats.Gets)
		t.Equal(uint64(0), drv.stats.Hits)
		time.Sleep(time.Second * 2)
		key, _ := drv.Hash(all, []any{1})
		t.True(drv.Cache.Has(context.Background(), drv.CachePrefix+string(key)), "entry key query ttl set no expired")
	})
	t.Run("refChanged", func() {
		drv := NewDriver(t.DB, WithConfiguration(conf.NewFromStringMap(map[string]any{
			"hashQueryTTL": time.Minute,
			"name":         "refChanged",
		})))
		query(drv, WithRefEntryKey(context.Background(), "User", 1), "SELECT age FROM users where id=?", []any{1})
		drv.ChangeSet.Store("User:1")
		query(drv, WithRefEntryKey(context.Background(), "User", 1), "SELECT age FROM users where id=?", []any{1})
		t.Equal(uint64(2), drv.stats.Gets)
		t.Equal(uint64(0), drv.stats.Hits, "first query will be evicted")
		query(drv, context.Background(), "SELECT age FROM users where id=?", []any{1})
		t.Equal(uint64(3), drv.stats.Gets)
		t.Equal(uint64(1), drv.stats.Hits, "common query should use the the cached")
		ctx := WithTTL(context.Background(), time.Second)
		query(drv, WithRefEntryKey(ctx, "User", 1), "SELECT age FROM users where id=?", []any{1})
		t.Equal(uint64(2), drv.stats.Hits)
	})
	t.Run("context", func() {
		drv := NewDriver(t.DB, WithConfiguration(conf.NewFromStringMap(map[string]any{
			"hashQueryTTL": time.Minute,
			"name":         "context",
		})))
		all := "SELECT * FROM users where id=?"
		ctx := WithTTL(context.Background(), time.Second)
		query(drv, WithEntryKey(ctx, "User", 1), all, []any{1})
		time.Sleep(time.Second * 2)
		query(drv, WithEntryKey(ctx, "User", 1), all, []any{1})
		t.Equal(uint64(0), drv.stats.Hits)
		drv.ChangeSet.Store("User:1")
		query(drv, WithEntryKey(ctx, "User", 1), all, []any{1})
		t.Len(drv.ChangeSet.changes, 0)

		query(drv, Evict(context.Background()), all, []any{1})
		t.Equal(uint64(0), drv.stats.Hits)
		key, _ := drv.Hash(all, []any{1})
		t.True(drv.Cache.Has(context.Background(), string(key)), "evict should refresh the cache")
		query(drv, Skip(ctx), all, []any{1})
		t.Equal(uint64(0), drv.stats.Hits)
	})
}

func (t *driverSuite) TestTx() {
	var dest struct {
		id  int
		age float64
	}
	drv := NewDriver(t.DB, WithConfiguration(conf.NewFromStringMap(map[string]any{
		"hashQueryTTL": time.Minute,
		"name":         "tx",
	})))
	ctx := context.Background()
	tx, err := drv.Tx(ctx)
	t.Require().NoError(err)
	t.NoError(tx.Exec(ctx, "insert into users values (?,?)", []any{2, 30.1}, nil))
	rows := &sql.Rows{}
	t.NoError(tx.Query(ctx, "SELECT age FROM users where id=?", []any{2}, rows))
	rows.Next()
	_ = rows.Scan(&dest.age)
	_ = rows.Close()
}

func (t *driverSuite) TestGC() {
	drv := NewDriver(t.DB, WithConfiguration(conf.NewFromStringMap(map[string]any{
		"hashQueryTTL": time.Second,
		"name":         "gc",
	})), WithChangeSet(NewChangeSet(time.Second*2)))
	ctx, concel := context.WithTimeout(context.Background(), time.Second*5)
	defer concel()
	go drv.ChangeSet.Start(ctx)
	drv.ChangeSet.Store("gc:1")
	drv.ChangeSet.Store("gc:2")
	drv.ChangeSet.LoadOrStoreRef("ref:1")
	drv.ChangeSet.LoadOrStoreRef("ref:2")
	time.Sleep(time.Second * 3)
	t.Equal(0, len(drv.ChangeSet.changes))
	t.Equal(0, len(drv.ChangeSet.refs))
}
