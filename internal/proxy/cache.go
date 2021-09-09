package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	vaultCacheTTL           = "VAULT_CACHE_TTL"
	queryParameterCacheable = "cacheable"
	queryParameterRecache   = "recache"
	queryParameterVersion   = "version"
)

type Cache struct {
	mu      sync.RWMutex
	data    map[CacheKey]CacheData
	timeout time.Duration
}

type CacheKey struct {
	Path    string
	Version string
}

type CacheData struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	expires    time.Time
}

func NewCache(timeout time.Duration) *Cache {
	return &Cache{
		data:    make(map[CacheKey]CacheData),
		timeout: timeout,
	}
}

func (c *Cache) Set(key CacheKey, data CacheData) {
	data.expires = time.Now().Add(c.timeout)
	c.mu.Lock()
	c.data[key] = data
	c.mu.Unlock()
}

func (c *Cache) Get(key CacheKey) (data CacheData, ok bool) {
	c.mu.RLock()
	data, ok = c.data[key]
	c.mu.RUnlock()
	if ok && time.Now().After(data.expires) {
		ok = false
		c.Remove(key)
	}
	return
}

func (c *Cache) Remove(key CacheKey) {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock() // defer is a bit slower then explicit call
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func setupCache() *Cache {
	cacheTTLEnv := os.Getenv(vaultCacheTTL)
	if cacheTTLEnv != "" {
		cacheTTL, err := time.ParseDuration(cacheTTLEnv)
		if err == nil && cacheTTL > 0 {
			return NewCache(cacheTTL)
		}
	}
	return nil
}

func shallFetchCache(r *http.Request, cache *Cache) bool {
	if cache == nil {
		return false
	}
	cacheable := r.URL.Query().Get(queryParameterCacheable)
	recache := r.URL.Query().Get(queryParameterRecache)
	return r.Method == "GET" && cacheable == "1" && recache != "1"
}

func shallRefreshCache(r *http.Request, cache *Cache) bool {
	if cache == nil {
		return false
	}
	cacheable := r.URL.Query().Get(queryParameterCacheable)
	return r.Method == "GET" && cacheable == "1"
}

func fetchFromCache(w http.ResponseWriter, data CacheData) {
	copyHeaders(w.Header(), data.Header)
	w.WriteHeader(data.StatusCode)
	w.Write(data.Body)
}

func retrieveData(resp *http.Response) CacheData {
	var data CacheData
	data.StatusCode = resp.StatusCode
	data.Header = resp.Header

	var buf bytes.Buffer
	_, err := io.Copy(&buf, resp.Body)
	if err != nil {
		data.StatusCode = http.StatusInternalServerError // also cache errors
		data.Body = []byte(fmt.Sprintf("failed to write response back to requester: %s", err))
	} else {
		data.Body = buf.Bytes()
	}
	return data
}
