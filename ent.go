package entcache

import (
	"context"
	"entgo.io/ent"
	"log/slog"
	"strconv"
)

type HookOption func(*hookOptions)

type hookOptions struct {
	// DriverName is the key of the cache.
	DriverName string
}

// WithDriverName sets which named ent cache driver name to use.
func WithDriverName(name string) HookOption {
	return func(options *hookOptions) {
		options.DriverName = name
	}
}

// DataChangeNotify returns a hook that notifies the cache when a mutation is performed.
//
// Driver in method is a placeholder for the cache driver name, which is lazy loaded by NewDriver.
// Use IDs method to get the ids of the mutation, that also works for XXXOne.
func DataChangeNotify(opts ...HookOption) ent.Hook {
	var options = hookOptions{
		DriverName: defaultDriverName,
	}
	for _, opt := range opts {
		opt(&options)
	}
	driver, ok := driverManager[options.DriverName]
	if !ok {
		driver = new(Driver)
		driverManager[options.DriverName] = driver
	}
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (v ent.Value, err error) {
			op := m.Op()
			if op.Is(ent.OpCreate) || driver.Config == nil {
				return next.Mutate(ctx, m)
			}
			var ids []int
			switch op {
			case ent.OpUpdateOne:
				if v, err = next.Mutate(ctx, m); err != nil {
					return nil, err
				}
				if ider, ok := m.(interface {
					ID() (id int, exists bool)
				}); ok {
					if id, ok := ider.ID(); ok {
						ids = []int{id}
					}
				}
			case ent.OpDeleteOne:
				if ider, ok := m.(interface {
					ID() (id int, exists bool)
				}); ok {
					if id, ok := ider.ID(); ok {
						ids = []int{id}
					}
				}
				if v, err = next.Mutate(ctx, m); err != nil {
					return nil, err
				}
			case ent.OpUpdate:
				if v, err = next.Mutate(ctx, m); err != nil {
					return nil, err
				}
				if ider, ok := m.(interface {
					IDs(ctx context.Context) ([]int, error)
				}); ok {
					ids, err = ider.IDs(ctx)
					if err != nil {
						slog.Error("EntCache getting ids", "err", err)
						return v, nil
					}
				}
			case ent.OpDelete:
				if ider, ok := m.(interface {
					IDs(ctx context.Context) ([]int, error)
				}); ok {
					ids, err = ider.IDs(ctx)
					if err != nil {
						slog.Error("EntCache getting ids", "err", err)
					}
				}
				if v, err = next.Mutate(ctx, m); err != nil {
					return nil, err
				}
			}
			if len(ids) > 0 {
				var keys = make([]Key, len(ids))
				for i, id := range ids {
					keys[i] = NewEntryKey(m.Type(), strconv.Itoa(id))
				}
				driver.ChangeSet.Store(keys...)
			}
			return v, err
		})
	}
}
