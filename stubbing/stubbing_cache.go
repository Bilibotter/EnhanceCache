package stubbing

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	caches = make([]*StubbingCache, 0)
	wakeup chan bool
	lock   sync.Mutex
	once   sync.Once
)

type StubbingCache struct {
	defaultExpiration time.Duration
	nextScan          time.Time
	items             sync.Map // just like map[string]*Item
}

type Item struct {
	Object     any
	status     int32
	Expiration time.Time
}

func NewCache(expired time.Duration) *StubbingCache {
	once.Do(func() {
		wakeup = make(chan bool, 1)
		go clearExpired()
	})

	cache := &StubbingCache{
		items:             sync.Map{},
		nextScan:          time.Now().Add(expired),
		defaultExpiration: expired,
	}

	lock.Lock()
	defer lock.Unlock()

	caches = append(caches, cache)
	// this will lead to clear the cache with a short expiration time first
	sort.SliceStable(caches, func(i, j int) bool {
		return caches[i].defaultExpiration < caches[j].defaultExpiration
	})

	select {
	case wakeup <- true:
	default:
	}

	return cache
}

func (ec *StubbingCache) Get(key string) (any, bool) {
	wrap, exist := ec.items.Load(key)

	if !exist {
		return nil, false
	}

	item := wrap.(*Item)
	if item.Expiration.After(time.Now()) {
		return item.Object, true
	}

	if atomic.CompareAndSwapInt32(&item.status, 0, 1) {
		ec.items.Delete(key)
		atomic.StoreInt32(&item.status, 2)
		return nil, false
	}

	// all operations need to wait for the deletion to complete
	// extreme short time to wait, only trigger in stubbing
	for status := atomic.LoadInt32(&item.status); status != 2; {
		status = atomic.LoadInt32(&item.status)
	}

	return nil, false
}

func (ec *StubbingCache) Delete(key string) {
	ec.Get(key)
	ec.items.Delete(key)
}

func (ec *StubbingCache) Set(key string, value any, expiration time.Duration) {
	ec.Get(key)

	item := &Item{
		Object:     value,
		Expiration: time.Now().Add(expiration),
	}

	ec.items.Store(key, item)
}

func (ec *StubbingCache) LoadOrStore(key string, value any) (any, bool) {
	if target, exist := ec.Get(key); exist {
		return target, false
	}

	item := &Item{
		Object:     value,
		Expiration: time.Now().Add(ec.defaultExpiration),
	}

	warp, load := ec.items.LoadOrStore(key, item)
	item = warp.(*Item)
	return item.Object, load
}

func clearExpired() {
	// wait for all caches to be initialized
	time.Sleep(10 * time.Second)
	for {
		lock.Lock()
		nearest := time.Now()
		for _, cache := range caches {
			if cache.nextScan.After(time.Now()) {
				continue
			}
			cache.items.Range(func(key, value any) bool {
				cache.Get(key.(string))
				return true
			})
			cache.nextScan = time.Now().Add(cache.defaultExpiration)
			if cache.nextScan.Before(nearest) {
				nearest = cache.nextScan
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
