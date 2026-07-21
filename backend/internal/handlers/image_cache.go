package handlers

import (
	"container/list"
	"sync"
	"time"
)

const (
	// 条目数和单条大小共同约束进程内缓存的内存上限。
	imageCacheMaxEntries         = 128
	imageCacheMaxBodyBytes int64 = 1 << 20
	imageCacheTTL                = 10 * time.Minute
	imageFetchConcurrency        = 16
	imageRedirectLimit           = 3
)

type imageCacheEntry struct {
	body        []byte
	contentType string
	expiresAt   time.Time
}

type imageCacheItem struct {
	key   string
	entry imageCacheEntry
}

// imageCache 是条目数、单条体积和有效期都受约束的 LRU 缓存。
type imageCache struct {
	mu      sync.Mutex
	order   *list.List
	entries map[string]*list.Element
}

func newImageCache() *imageCache {
	return &imageCache{order: list.New(), entries: make(map[string]*list.Element)}
}

func (c *imageCache) get(key string) (imageCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return imageCacheEntry{}, false
	}
	item := element.Value.(*imageCacheItem)
	if time.Now().After(item.entry.expiresAt) {
		c.order.Remove(element)
		delete(c.entries, key)
		return imageCacheEntry{}, false
	}
	c.order.MoveToFront(element)
	return item.entry, true
}

func (c *imageCache) put(key string, entry imageCacheEntry) {
	if int64(len(entry.body)) > imageCacheMaxBodyBytes {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[key]; ok {
		element.Value.(*imageCacheItem).entry = entry
		c.order.MoveToFront(element)
		return
	}

	c.entries[key] = c.order.PushFront(&imageCacheItem{key: key, entry: entry})
	for c.order.Len() > imageCacheMaxEntries {
		oldest := c.order.Back()
		if oldest == nil {
			return
		}
		oldestItem := c.order.Remove(oldest).(*imageCacheItem)
		delete(c.entries, oldestItem.key)
	}
}

type imageProxyError struct {
	status  int
	code    string
	message string
}

func (e *imageProxyError) Error() string { return e.message }

type imageFetchCall struct {
	done      chan struct{}
	entry     imageCacheEntry
	failure   *imageProxyError
	transport bool
}

// imageSingleFlight 合并同一 URL 的并发抓取，避免首屏重复请求同一封面。
type imageSingleFlight struct {
	mu    sync.Mutex
	calls map[string]*imageFetchCall
}

func newImageSingleFlight() *imageSingleFlight {
	return &imageSingleFlight{calls: make(map[string]*imageFetchCall)}
}

func (s *imageSingleFlight) do(
	key string,
	fn func() (imageCacheEntry, *imageProxyError, bool),
) (imageCacheEntry, *imageProxyError, bool) {
	s.mu.Lock()
	if call, ok := s.calls[key]; ok {
		s.mu.Unlock()
		<-call.done
		return call.entry, call.failure, call.transport
	}

	call := &imageFetchCall{done: make(chan struct{})}
	s.calls[key] = call
	s.mu.Unlock()

	entry, failure, transport := fn()
	call.entry, call.failure, call.transport = entry, failure, transport

	s.mu.Lock()
	delete(s.calls, key)
	close(call.done)
	s.mu.Unlock()
	return entry, failure, transport
}
