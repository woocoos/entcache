package entcache

import (
	"context"
	stdsql "database/sql"
	"database/sql/driver"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"errors"
	"fmt"
	"github.com/tsingsun/woocoo/pkg/cache"
	"github.com/tsingsun/woocoo/pkg/cache/lfu"
	"github.com/tsingsun/woocoo/pkg/conf"
	"github.com/tsingsun/woocoo/pkg/log"
	"strings"
	"sync/atomic"
	"time"
	_ "unsafe"
)

//go:linkname convertAssign database/sql.convertAssign
func convertAssign(dest, src any) error

const (
	defaultDriverName = "default"
	defaultGCInterval = time.Hour
)

var (
	// errSkip tells the driver to skip cache layer.
	errSkip = errors.New("entcache: skip cache")

	driverManager = make(map[string]*Driver)
	logger        = log.Component("entcache")
)

type (
	// A Driver is a SQL cached client. Users should use the
	// constructor below for creating a new driver.
	Driver struct {
		*Config
		dialect.Driver
		stats Stats

		Hash func(query string, args []any) (Key, error)
	}
	// Stats represent the cache statistics of the driver.
	Stats struct {
		Gets   uint64
		Hits   uint64
		Errors uint64
	}
)

// NewDriver wraps the given driver with a caching layer.
func NewDriver(drv dialect.Driver, opts ...Option) *Driver {
	options := &Config{
		Name:        defaultDriverName,
		GCInterval:  defaultGCInterval,
		KeyQueryTTL: defaultGCInterval,
	}
	for _, opt := range opts {
		opt(options)
	}
	var d *Driver
	d, ok := driverManager[options.Name]
	if !ok {
		d = &Driver{}
		driverManager[options.Name] = d
	}
	d.Config = options
	if d.Config.Cache == nil {
		if d.Config.StoreKey != "" {
			var err error
			d.Cache, err = cache.GetCache(d.Config.StoreKey)
			if err != nil {
				panic(err)
			}
		} else {
			cnf := conf.NewFromStringMap(map[string]any{
				"size": 10000,
			})
			if d.Config.HashQueryTTL > 0 {
				cnf.Parser().Set("ttl", d.Config.HashQueryTTL)
			}
			c, err := lfu.NewTinyLFU(cnf)
			if err != nil {
				panic(err)
			}
			d.Cache = c
		}
	}
	d.Driver = drv
	d.Hash = DefaultHash
	if d.ChangeSet == nil {
		d.ChangeSet = NewChangeSet(d.GCInterval)
	}
	return d
}

// Query implements the Querier interface for the driver. It falls back to the
// underlying wrapped driver in case of caching error.
//
// Note that the driver does not synchronize identical queries that are executed
// concurrently. Hence, if 2 identical queries are executed at the ~same time, and
// there is no cache entry for them, the driver will execute both of them and the
// last successful one will be stored in the cache.
func (d *Driver) Query(ctx context.Context, query string, args, v any) error {
	// Check if the given statement looks like a standard Ent query (e.g. SELECT).
	// Custom queries (e.g. CTE) or statements that are prefixed with comments are
	// not supported. This check is mainly necessary, because PostgreSQL and SQLite
	// may execute an insert statement like "INSERT ... RETURNING" using Driver.Query.
	if !strings.HasPrefix(query, "SELECT") && !strings.HasPrefix(query, "select") {
		return d.Driver.Query(ctx, query, args, v)
	}
	vr, ok := v.(*sql.Rows)
	if !ok {
		return fmt.Errorf("entcache: invalid type %T. expect *sql.Rows", v)
	}
	argv, ok := args.([]any)
	if !ok {
		return fmt.Errorf("entcache: invalid type %T. expect []interface{} for args", args)
	}
	opts, err := d.optionsFromContext(ctx, query, argv)
	if err != nil {
		return d.Driver.Query(ctx, query, args, v)
	}
	atomic.AddUint64(&d.stats.Gets, 1)
	var e Entry
	if opts.evict {
		err = cache.ErrCacheMiss
	} else {
		err = d.Cache.Get(ctx, string(opts.key), &e, cache.WithSkip(opts.skipMode))
	}
	switch {
	case err == nil:
		atomic.AddUint64(&d.stats.Hits, 1)
		vr.ColumnScanner = &repeater{columns: e.Columns, values: e.Values}
	case errors.Is(err, cache.ErrCacheMiss):
		if err := d.Driver.Query(ctx, query, args, vr); err != nil {
			return err
		}
		vr.ColumnScanner = &recorder{
			ColumnScanner: vr.ColumnScanner,
			onClose: func(columns []string, values [][]driver.Value) {
				if opts.skipNotFound && len(values) == 0 {
					return
				}
				err := d.Cache.Set(ctx, string(opts.key), &Entry{Columns: columns, Values: values},
					cache.WithTTL(opts.ttl), cache.WithSkip(opts.skipMode),
				)
				if err != nil {
					atomic.AddUint64(&d.stats.Errors, 1)
					logger.Warn(fmt.Sprintf("entcache: failed storing entry %v in cache: %v", opts.key, err))
				}
			},
		}
	default:
		return d.Driver.Query(ctx, query, args, v)
	}
	return nil
}

// optionsFromContext returns the injected options from the context, or its default value.
// Note that the key in the context is an entry key, and will replace by hashed query key, that will improve the cache hit rate.
func (d *Driver) optionsFromContext(ctx context.Context, query string, args []any) (ctxOptions, error) {
	var opts ctxOptions
	if c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions); ok {
		opts = *c
		if c.key != "" {
			c.key = "" // clear it for eager loading.
		}
	}
	key, err := d.Hash(query, args)
	if err != nil {
		return opts, errSkip
	}
	switch {
	case opts.ref && opts.key != "":
		if t, ok := d.ChangeSet.Load(opts.key); ok {
			rt, loaded := d.ChangeSet.LoadOrStoreRef(key)
			// the first query in the entity changed period, evict the cache;
			// if the new entity changed happen after the previous query, evict the cache
			opts.evict = !loaded || t.After(rt)
		} else if _, ok := d.ChangeSet.LoadRef(key); ok {
			opts.evict = true
			d.ChangeSet.DeleteRef(key)
		}
		if opts.ttl == 0 {
			opts.ttl = d.KeyQueryTTL
		}
	case opts.key == "":
		if opts.ttl == 0 {
			opts.ttl = d.HashQueryTTL
		}
	case opts.key != "":
		if _, ok := d.ChangeSet.Load(opts.key); ok {
			opts.evict = true
			d.ChangeSet.Delete(opts.key)
		}
		if opts.ttl == 0 {
			opts.ttl = d.KeyQueryTTL
		}
	}
	// use hashed key as the cache key
	opts.key = key
	if d.CachePrefix != "" {
		opts.key = Key(d.CachePrefix) + opts.key
	}
	if opts.skipMode == cache.SkipCache {
		return opts, errSkip
	}
	return opts, nil
}

// rawCopy copies the driver values by implementing
// the sql.Scanner interface.
type rawCopy struct {
	values []driver.Value
}

func (c *rawCopy) Scan(src interface{}) error {
	if b, ok := src.([]byte); ok {
		b1 := make([]byte, len(b))
		copy(b1, b)
		src = b1
	}
	c.values[0] = src
	c.values = c.values[1:]
	return nil
}

// recorder represents an sql.Rows recorder that implements
// the entgo.io/ent/dialect/sql.ColumnScanner interface.
type recorder struct {
	sql.ColumnScanner
	values  [][]driver.Value
	columns []string
	done    bool
	onClose func([]string, [][]driver.Value)
}

// Next wraps the underlying Next method
func (r *recorder) Next() bool {
	hasNext := r.ColumnScanner.Next()
	r.done = !hasNext
	return hasNext
}

// Scan copies database values for future use (by the repeater)
// and assign them to the given destinations using the standard
// database/sql.convertAssign function.
func (r *recorder) Scan(dest ...any) error {
	values := make([]driver.Value, len(dest))
	args := make([]any, len(dest))
	c := &rawCopy{values: values}
	for i := range args {
		args[i] = c
	}
	if err := r.ColumnScanner.Scan(args...); err != nil {
		return err
	}
	for i := range values {
		if err := convertAssign(dest[i], values[i]); err != nil {
			return err
		}
	}
	r.values = append(r.values, values)
	return nil
}

// Columns wraps the underlying Column method and stores it in the recorder state.
// The repeater.Columns cannot be called if the recorder method was not called before.
// That means, raw scanning should be identical for identical queries.
func (r *recorder) Columns() ([]string, error) {
	columns, err := r.ColumnScanner.Columns()
	if err != nil {
		return nil, err
	}
	r.columns = columns
	return columns, nil
}

func (r *recorder) Close() error {
	if err := r.ColumnScanner.Close(); err != nil {
		return err
	}
	// If we did not encounter any error during iteration,
	// and we scanned all rows, we store it on cache.
	if err := r.ColumnScanner.Err(); err == nil || r.done {
		r.onClose(r.columns, r.values)
	}
	return nil
}

// repeater repeats columns scanning from cache history.
type repeater struct {
	columns []string
	values  [][]driver.Value
}

func (*repeater) Close() error {
	return nil
}
func (*repeater) ColumnTypes() ([]*stdsql.ColumnType, error) {
	return nil, fmt.Errorf("entcache.ColumnTypes is not supported")
}
func (r *repeater) Columns() ([]string, error) {
	return r.columns, nil
}
func (*repeater) Err() error {
	return nil
}
func (r *repeater) Next() bool {
	return len(r.values) > 0
}

func (r *repeater) NextResultSet() bool {
	return len(r.values) > 0
}

func (r *repeater) Scan(dest ...any) error {
	if !r.Next() {
		return stdsql.ErrNoRows
	}
	for i, src := range r.values[0] {
		if err := convertAssign(dest[i], src); err != nil {
			return err
		}
	}
	r.values = r.values[1:]
	return nil
}
