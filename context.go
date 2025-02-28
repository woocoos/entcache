package entcache

import (
	"context"
	"fmt"
	"github.com/tsingsun/woocoo/pkg/cache"
	"time"
)

// ctxOptions allows injecting runtime options.
type ctxOptions struct {
	evict        bool           // i.e. skip and invalidate entry.
	skipNotFound bool           // skip cache if query return 0 row.
	key          Key            // entry key.
	ref          bool           // indicates if the key is a reference key.
	ttl          time.Duration  // entry duration.
	skipMode     cache.SkipMode // skip mode
}

var ctxOptionsKey ctxOptions

// Skip returns a new Context that tells the Driver
// to skip the cache entry on Query.
//
//	client.T.Query().All(entcache.Skip(ctx))
func Skip(ctx context.Context) context.Context {
	c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions)
	if !ok {
		return context.WithValue(ctx, ctxOptionsKey, &ctxOptions{skipMode: cache.SkipCache})
	}
	c.skipMode = cache.SkipCache
	return ctx
}

// Evict returns a new Context that tells the Driver to refresh the cache entry on Query.
//
//	client.T.Query().All(entcache.Evict(ctx))
func Evict(ctx context.Context) context.Context {
	c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions)
	if !ok {
		return context.WithValue(ctx, ctxOptionsKey, &ctxOptions{evict: true})
	}
	c.evict = true
	return ctx
}

// SkipNotFound returns a new Context that tells the Driver to ignore cache if zero row return.
//
//	client.T.Query().All(entcache.SkipNotFound(ctx))
func SkipNotFound(ctx context.Context) context.Context {
	c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions)
	if !ok {
		return context.WithValue(ctx, ctxOptionsKey, &ctxOptions{skipNotFound: true})
	}
	c.skipNotFound = true
	return ctx
}

// WithEntryKey returns a new Context that carries the Key for the cache entry.
// Note that the key is one shot, otherwise cause error if the ent.Client query involves
// more than 1 SQL query (e.g. eager loading).
func WithEntryKey(ctx context.Context, typ string, id any) context.Context {
	key := NewEntryKey(typ, fmt.Sprint(id))
	c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions)
	if !ok {
		return context.WithValue(ctx, ctxOptionsKey, &ctxOptions{key: key})
	}
	c.key = key
	return ctx
}

// WithRefEntryKey returns a new Context that carries a reference Entry Key for the cache entry.
// RefEntryKey indicates if the key is a reference an entry key. For example, when Get is called, ref is false, because Get use
// id query and get all fields. When others are called, such as Only, ref is false.
func WithRefEntryKey(ctx context.Context, typ string, id any) context.Context {
	key := NewEntryKey(typ, fmt.Sprint(id))
	c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions)
	if !ok {
		return context.WithValue(ctx, ctxOptionsKey, &ctxOptions{key: key, ref: true})
	}
	c.key = key
	c.ref = true
	return ctx
}

// WithTTL returns a new Context that carries the TTL for the cache entry.
//
//	client.T.Query().All(entcache.WithTTL(ctx, time.Second))
func WithTTL(ctx context.Context, ttl time.Duration) context.Context {
	c, ok := ctx.Value(ctxOptionsKey).(*ctxOptions)
	if !ok {
		return context.WithValue(ctx, ctxOptionsKey, &ctxOptions{ttl: ttl})
	}
	c.ttl = ttl
	return ctx
}
