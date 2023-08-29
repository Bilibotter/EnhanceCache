package core

import (
	"sync"
	"sync/atomic"
	"time"
)

var (
	caches = make([]any, 0)
	wakeup chan bool
	lock   sync.Mutex
	once   sync.Once
)

type Cleanable interface {
	clean() (nextCleanTime time.Time)
}

type EnhanceCache[K string, V any] struct {
	defaultExpiration time.Duration
	nextScan          time.Time
	items             sync.Map // just like map[string]*Item
}

type Item struct {
	Object     any
	status     int32
	Expiration time.Time
}

func NewCache[V any](expired time.Duration) *EnhanceCache[string, V] {
	once.Do(func() {
		wakeup = make(chan bool, 1)
		go clearExpired()
	})

	cache := &EnhanceCache[string, V]{
		items:             sync.Map{},
		nextScan:          time.Now().Add(expired),
		defaultExpiration: expired,
	}

	lock.Lock()
	defer lock.Unlock()

	caches = append(caches, cache)
	//// this will lead to clear the cache with a short expiration time first
	//sort.SliceStable(caches, func(i, j int) bool {
	//	c1 := caches[i].(*EnhanceCache[string, V])
	//	c2 := caches[i].(*EnhanceCache[string, V])
	//	return caches[i].defaultExpiration < caches[j].defaultExpiration
	//})

	select {
	case wakeup <- true:
	default:
	}

	return cache
}

func (ec *EnhanceCache[K, V]) Get(key string) (v V, exist bool) {
	wrap, exist := ec.items.Load(key)

	if !exist {
		return
	}

	item := wrap.(*Item)
	if item.Expiration.After(time.Now()) {
		return item.Object, true
	}

	if atomic.CompareAndSwapInt32(&item.status, 0, 1) {
		ec.items.Delete(key)
		atomic.StoreInt32(&item.status, 2)
		return
	}

	// all operations need to wait for the deletion to complete
	// extreme short time to wait, only trigger in test
	for status := atomic.LoadInt32(&item.status); status != 2; {
		status = atomic.LoadInt32(&item.status)
	}

	return
}

func (ec *EnhanceCache[K, V]) Delete(key string) {
	ec.Get(key)
	ec.items.Delete(key)
}

func (ec *EnhanceCache[K, V]) Set(key string, value V, expiration time.Duration) {
	ec.Get(key)

	item := &Item{
		Object:     value,
		Expiration: time.Now().Add(expiration),
	}

	ec.items.Store(key, item)
}

func (ec *EnhanceCache[K, V]) LoadOrStore(key string, value V) (V, bool) {
	if target, exist := ec.Get(key); exist {
		return target, false
	}

	item := &Item{
		Object:     value,
		Expiration: time.Now().Add(ec.defaultExpiration),
	}

	warp, load := ec.items.LoadOrStore(key, item)
	item = warp.(*Item)
	return item.Object.(V), load
}

func (ec *EnhanceCache[K, V]) clean() (nextCleanTime time.Time) {
	if ec.nextScan.After(time.Now()) {
		return ec.nextScan
	}
	ec.items.Range(func(key, value any) bool {
		ec.Get(key.(string))
		return true
	})
	ec.nextScan = time.Now().Add(ec.defaultExpiration)
	return ec.nextScan
}

func (ec *EnhanceCache[K, V]) Flush() {
	ec.items = sync.Map{}
}

func clearExpired() {
	// wait for all caches to be initialized
	time.Sleep(10 * time.Second)
	for {
		lock.Lock()
		nearest := time.Now().Add(time.Hour)
		for _, cache := range caches {
			clearer, ok := cache.(Cleanable)
			if !ok {
				panic("cache must implement Clearable interface")
			}
			nextScan := clearer.clean()
			if nextScan.Before(nearest) {
				nearest = nextScan
			}
		}
		// it's unsafe to use defer to unlock in loop
		lock.Unlock()
		// if one goroutine can't clear faster than write,
		// the program may have OOM exception
		// so just need a goroutine to clear
		if time.Now().After(nearest) {
			continue
		}
		if nearest.Sub(time.Now()) > time.Hour {
			nearest = time.Now().Add(time.Hour)
		}

		timer := time.NewTimer(nearest.Sub(time.Now()))
		select {
		case <-timer.C:
			continue
		case <-wakeup:
			// if create cache during clearExpired,
			// it needs to start clear immediately
			continue
		default:
		}
	}
}
