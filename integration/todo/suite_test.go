package todo

import (
	"context"
	"entgo.io/contrib/entgql"
	"entgo.io/ent/dialect/sql"
	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/suite"
	"github.com/tsingsun/woocoo/pkg/cache/redisc"
	"github.com/tsingsun/woocoo/pkg/conf"
	"github.com/woocoos/entcache"
	"github.com/woocoos/entcache/integration/todo/ent"
	"github.com/woocoos/entcache/integration/todo/ent/enttest"
	"github.com/woocoos/entcache/integration/todo/ent/migrate"
	"github.com/woocoos/entcache/integration/todo/ent/todo"
	"github.com/woocoos/entcache/integration/todo/ent/user"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/woocoos/entcache/integration/todo/ent/runtime"
)

const (
	queryAll = `query {
		todos {
			totalCount
			edges {
				node {
					id
					status
				}
				cursor
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
				startCursor
				endCursor
			}
		}
	}`
	nodeTodo = `query {
		node(id: 1) {
			__typename
			... on Todo {
				id
				status
			}
		}
	}`
)

type Suite struct {
	suite.Suite
	nativeDriver *sql.Driver
	cacheDriver  *entcache.Driver
	ent          *ent.Client
	Redis        *miniredis.Miniredis
	gqlClient    *client.Client
}

func (s *Suite) SetupSuite() {
	drv, err := sql.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	s.Require().NoError(err)
	s.nativeDriver = drv

	s.Redis, err = miniredis.Run()
	s.Require().NoError(err)

	_, err = redisc.New(conf.NewFromStringMap(map[string]any{
		"driverName": "redis",
		"addrs":      []string{s.Redis.Addr()},
		"local": map[string]any{
			"size":    1000,
			"samples": 100000,
			"ttl":     "1m",
		},
	}))
	s.Require().NoError(err)

	s.cacheDriver = entcache.NewDriver(drv, entcache.WithConfiguration(conf.NewFromStringMap(
		map[string]any{
			"driverName": "redis",
		})))

	s.ent = enttest.NewClient(s.T(), enttest.WithOptions(ent.Driver(s.cacheDriver), ent.Debug()),
		enttest.WithMigrateOptions(migrate.WithGlobalUniqueID(true)),
	)
	s.mockData()
	srv := handler.NewDefaultServer(NewSchema(s.ent))
	srv.Use(entgql.Transactioner{TxOpener: s.ent})
	s.gqlClient = client.New(srv)
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) mockData() {
	tr := s.ent.Todo.Create().SetStatus(todo.StatusInProgress).SetText("text").SaveX(context.Background())
	ur := s.ent.User.Create().AddTodos(tr).SetName("name").SetAge(10).SaveX(context.Background())
	tr.Update().SetOwner(ur).SaveX(context.Background())
}

func (s *Suite) TestCache() {
	client := s.ent
	ctx := context.Background()
	row, err := client.User.Create().SetName("user1").Save(ctx)
	s.Require().NoError(err)
	u, err := client.User.Get(ctx, row.ID)
	s.Require().NoError(err)
	row, _ = client.User.UpdateOneID(row.ID).SetName("user2").Save(ctx)
	u, err = client.User.Get(ctx, row.ID)
	s.Equal("user2", u.Name)
	u, err = client.User.Get(ctx, row.ID)
}

func (s *Suite) TestPartialField() {
	ctx := context.Background()
	us := s.ent.User.Query().AllX(ctx)
	u, _ := s.ent.User.Get(ctx, us[0].ID)
	ctx = entcache.WithEntryKey(ctx, "User", u.ID)
	r := s.ent.User.Query().WithTodos().Where(user.ID(u.ID)).Select("name").OnlyX(ctx)
	s.Equal(u.Name, r.Name)
	r = s.ent.User.Query().Where(user.ID(u.ID)).Select("name").OnlyX(context.Background())
	s.Equal(u.Name, r.Name)

	s.ent.Todo.Query().WithParent().Where(todo.ID(1)).OnlyX(ctx)
}

func (s *Suite) TestGraphqlQuery() {
	_, err := s.gqlClient.RawPost(queryAll)
	s.Require().NoError(err)
}

func (s *Suite) TestGraphqlNode() {
	resp, err := s.gqlClient.RawPost(nodeTodo)
	s.Require().NoError(err)
	s.Empty(string(resp.Errors))
}

func (s *Suite) Test_DeleteUser() {
	ctx := context.Background()
	us := s.ent.User.Query().AllX(ctx)
	u, _ := s.ent.User.Get(ctx, us[0].ID)
	s.ent.User.DeleteOneID(u.ID).ExecX(ctx)
}
