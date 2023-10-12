package entcache

import (
	"github.com/mitchellh/hashstructure/v2"
	"github.com/tsingsun/woocoo/pkg/cache"
	"github.com/tsingsun/woocoo/pkg/conf"
	"strconv"
	"time"
)

type (
	// Config wraps the basic configuration cache options.
	Config struct {
		// Name of the driver, used for ent cache driver mandger.
		Name string `yaml:"name" json:"name"`
		// Cache defines the cache implementation for holding the cache entries.
		// Default is tinyLFU with size 100000 and HashQueryTTL 1 minute.
		Cache cache.Cache `yaml:"-" json:"-"`
		// HashQueryTTL defines the period of time that an Entry that is hashed through by not Get
		// is valid in the cache.
		HashQueryTTL time.Duration `yaml:"hashQueryTTL" json:"hashQueryTTL"`
		// KeyQueryTTL defines the period of time that an Entry that is not hashed through by Get. This is keep the cached
		// data fresh, can be set to long time, such as 1 hour.
		KeyQueryTTL time.Duration `yaml:"keyQueryTTL" json:"keyQueryTTL"`
		// GCInterval defines the period of time that the cache will be GC.
		GCInterval time.Duration `yaml:"gcInterval" json:"gcInterval"`
		// StoreKey is the driver name of cache driver
		StoreKey string `yaml:"storeKey" json:"storeKey"`
		// CachePrefix is the prefix of cache key, avoid key conflict in redis cache
		CachePrefix string `yaml:"cachePrefix" json:"cachePrefix"`
		// ChangeSet manages data change
		ChangeSet *ChangeSet
	}

	// Option allows configuring the cache
	// driver using functional options.
	Option func(*Config)
)

func WithChangeSet(cs *ChangeSet) Option {
	return func(c *Config) {
		c.ChangeSet = cs
	}
}

// WithCache provides a cache implementation for holding the cache entries.
func WithCache(cc cache.Cache) Option {
	return func(c *Config) {
		c.Cache = cc
	}
}

// WithConfiguration provides a configuration option for the cache driver.
func WithConfiguration(cnf *conf.Configuration) Option {
	return func(c *Config) {
		if err := cnf.Unmarshal(c); err != nil {
			panic(err)
		}
	}
}

// DefaultHash provides the default implementation for converting
// a query and its argument to a cache key.
func DefaultHash(query string, args []any) (Key, error) {
	key, err := hashstructure.Hash(struct {
		Q string
		A []any
	}{
		Q: query,
		A: args,
	}, hashstructure.FormatV2, nil)
	if err != nil {
		return "", err
	}
	return Key(strconv.FormatUint(key, 10)), nil
}
