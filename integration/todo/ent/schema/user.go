package schema

import (
	"context"
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/woocoos/entcache"
	genent "github.com/woocoos/entcache/integration/todo/ent"
	"github.com/woocoos/entcache/integration/todo/ent/hook"
	"github.com/woocoos/entcache/integration/todo/ent/todo"
	"github.com/woocoos/entcache/integration/todo/ent/user"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "users"},
		entgql.QueryField(),
		entgql.RelayConnection(),
		entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate()),
	}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Float("age").Optional(),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("todos", Todo.Type),
	}
}

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		entcache.DataChangeNotify(),
		hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *genent.UserMutation) (genent.Value, error) {
				id, _ := m.ID()
				m.Client().Todo.Delete().Where(todo.HasOwnerWith(user.ID(id))).ExecX(ctx)
				return next.Mutate(ctx, m)
			})
		}, ent.OpDeleteOne),
	}
}

func (User) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		ent.TraverseFunc(func(ctx context.Context, query ent.Query) error {
			return nil
		}),
		ent.InterceptFunc(func(querier ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
				v, err := querier.Query(ctx, query)
				return v, err
			})
		}),
	}
}
