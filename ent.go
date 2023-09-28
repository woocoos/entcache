package entcache

import (
	"context"
	"entgo.io/ent"
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
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) {
				return next.Mutate(ctx, m)
			}
			v, err := next.Mutate(ctx, m)
			if err == nil {
				ider, ok := m.(interface {
					IDs(ctx context.Context) ([]int, error)
				})
				if ok {
					ids, err := ider.IDs(ctx)
					if err != nil {
						return nil, err
					}
					tn := m.Type()
					var keys = make([]Key, len(ids))
					for i, id := range ids {
						keys[i] = NewEntryKey(tn, strconv.Itoa(id))
					}
					driver.ChangeSet.Store(keys...)
				}
			}
			return v, err
		})
	}
}
