package handlers

import (
	"container/list"
	"sync"
	"time"
)

const (
	// imageCacheMaxEntries 限制进程内图片缓存的条目数，与 imageCacheMaxBodyBytes
	// 共同把内存占用约束在固定上限内（最大 maxEntries × maxBodyBytes）。
	imageCacheMaxEntries = 128
	// imageCacheTTL 是单条缓存的新鲜期，超时后即视为过期需重新校验抓取。
	imageCacheTTL = 10 * time.Minute
	// imageCacheMaxBodyBytes 限制可入库缓存的单张图片大小；超过该阈值的图片仍正常
	// 返回，但不进缓存，避免少数大图撑大缓存内存。
	imageCacheMaxBodyBytes int64 = 1 << 20 // 1 MiB
	// imageFetchConcurrency 限制并发上游图片抓取数量，作为有界队列防止突发请求耗尽连接或内存。
	imageFetchConcurrency = 16
	// imageRedirectLimit 限制逐跳校验后最多跟随的重定向次数。
	imageRedirectLimit = 3
	// imageProxyCacheControl 是回写给浏览器/CDN 的有界缓存指令，封面对该时长内可复用。
	imageProxyCacheControl = "public, max-age=3600"
)

// imageProxyError 承载图片代理中非传输类失败（超限、不支持的类型、主体不匹配等）
// 的 HTTP 状态码与稳定错误码，由 fetchImage 产生、ProxyImage 解释。
type imageProxyError struct {
	status  int
	code    string
	message string
}

func (e *imageProxyError) Error() string { return e.message }

// imageCacheEntry 持有一份已通过全部校验的上游图片主体、嗅探出的内容类型与新鲜截止时间。
type imageCacheEntry struct {
	body        []byte
	contentType string
	expiresAt   time.Time
}

type imageCacheItem struct {
	key   string
	entry imageCacheEntry
}

// imageCache 是条目数与单条体积都受约束的 LRU 缓存。
type imageCache struct {
	mu      sync.Mutex
	order   *list.List
	entries map[string]*list.Element
}

func newImageCache() *imageCache {
	return &imageCache{
		order:   list.New(),
		entries: make(map[string]*list.Element),
	}
}

func (c *imageCache) get(key string) (imageCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[key]
	if !ok {
		return imageCacheEntry{}, false
	}
	item := el.Value.(*imageCacheItem)
	if time.Now().After(item.entry.expiresAt) {
		c.order.Remove(el)
		delete(c.entries, key)
		return imageCacheEntry{}, false
	}
	c.order.MoveToFront(el)
	return item.entry, true
}

func (c *imageCache) put(key string, entry imageCacheEntry) {
	// 超过单条上限的图片不缓存，保证内存占用受约束。
	if int64(len(entry.body)) > imageCacheMaxBodyBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[key]; ok {
		el.Value.(*imageCacheItem).entry = entry
		c.order.MoveToFront(el)
		return
	}
	c.entries[key] = c.order.PushFront(&imageCacheItem{key: key, entry: entry})
	for c.order.Len() > imageCacheMaxEntries {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		oldestItem := c.order.Remove(oldest).(*imageCacheItem)
		delete(c.entries, oldestItem.key)
	}
}

// imageFetchCall 表示一次按 URL 合并的在途（或已完成）抓取；相同 URL 的并发调用者共享同一个 call。
type imageFetchCall struct {
	done      chan struct{}
	entry     imageCacheEntry
	failure   *imageProxyError
	transport bool // 为 true 表示上游抓取本身失败（网络/重定向被拒等），由 ProxyImage 映射为 502
}

// imageSingleFlight 合并同一 URL 的并发抓取，使突发重复请求只产生一次上游访问。
// 内联实现，避免引入 golang.org/x/sync 等新模块依赖。
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

	defer func() {
		close(call.done)
		s.mu.Lock()
		delete(s.calls, key)
		s.mu.Unlock()
	}()

	entry, failure, transport := fn()
	call.entry, call.failure, call.transport = entry, failure, transport
	return entry, failure, transport
}
