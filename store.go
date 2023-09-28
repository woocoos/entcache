package entcache

import (
	"context"
	"database/sql/driver"
	"sync"
	"time"
)

type (
	Entry struct {
		Columns []string
		Values  [][]driver.Value
	}

	Key string
)

func NewEntryKey(typ string, id string) Key {
	return Key(typ + ":" + id)
}

// ChangeSet is a set of keys that have changed, include update, delete, create.
type ChangeSet struct {
	sync.RWMutex
	changes    map[Key]time.Time
	refs       map[Key]time.Time
	gcInterval time.Duration
}

func NewChangeSet(gcInterval time.Duration) *ChangeSet {
	a := &ChangeSet{
		changes:    make(map[Key]time.Time),
		refs:       make(map[Key]time.Time),
		gcInterval: gcInterval,
	}
	if a.gcInterval <= 0 {
		a.gcInterval = defaultGCInterval
	}
	return a
}

func (a *ChangeSet) Start(ctx context.Context) error {
	t := time.NewTicker(a.gcInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			a.gc()
		}
	}
}

func (a *ChangeSet) Stop(ctx context.Context) error {
	return nil
}

func (a *ChangeSet) gc() {
	a.Lock()
	defer a.Unlock()
	for k, v := range a.changes {
		if time.Since(v) > a.gcInterval {
			delete(a.changes, k)
		}
	}
	for k, v := range a.refs {
		if time.Since(v) > a.gcInterval {
			delete(a.refs, k)
		}
	}
}

func (a *ChangeSet) Store(keys ...Key) {
	a.Lock()
	defer a.Unlock()
	t := time.Now()
	for _, key := range keys {
		a.changes[key] = t
	}
}

func (a *ChangeSet) Load(key Key) (time.Time, bool) {
	a.RLock()
	defer a.RUnlock()

	v, ok := a.changes[key]
	return v, ok
}

func (a *ChangeSet) Delete(key Key) {
	a.Lock()
	defer a.Unlock()

	delete(a.changes, key)
}

func (a *ChangeSet) LoadRef(key Key) (time.Time, bool) {
	a.RLock()
	defer a.RUnlock()

	v, ok := a.refs[key]
	return v, ok
}

// LoadOrStoreRef returns the time when the key was last updated.
func (a *ChangeSet) LoadOrStoreRef(key Key) (t time.Time, loaded bool) {
	a.Lock()
	defer a.Unlock()

	t, loaded = a.refs[key]
	a.refs[key] = time.Now()
	return
}

func (a *ChangeSet) DeleteRef(key Key) {
	a.Lock()
	defer a.Unlock()

	delete(a.refs, key)
}
