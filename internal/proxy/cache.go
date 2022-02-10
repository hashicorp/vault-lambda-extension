package proxy

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/vault/sdk/helper/cryptoutil"
	"github.com/hashicorp/vault/sdk/helper/strutil"
	gocache "github.com/patrickmn/go-cache"
)

const (
	vaultCacheTTL = "VAULT_DEFAULT_CACHE_TTL"

	// Return a cached response if it exists, otherwise fall back to Vault and
	// cache the response
	headerOptionCacheable = "cache"

	// Send the request to Vault and cache the response
	headerOptionRecache = "recache"
)

type Cache struct {
	data    *gocache.Cache
	timeout time.Duration
}

type CacheKey struct {
	Token       string
	Request     *http.Request
	RequestBody []byte
}

type CacheData struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type CacheOptions struct {
	cacheable bool
	recache   bool
}

func NewCache(timeout time.Duration) *Cache {
	return &Cache{
		data:    gocache.New(timeout, timeout),
		timeout: timeout,
	}
}

// constructs the CacheKey for this request and token and returns the SHA256
// hash
func makeRequestHash(logger *log.Logger, r *http.Request, token string) (string, error) {
	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read request body: %w", err)
	}
	if r.Body != nil {
		r.Body.Close()
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(reqBody))
	cacheKey := &CacheKey{
		Token:       token,
		Request:     r,
		RequestBody: reqBody,
	}

	cacheKeyHash, err := computeRequestID(cacheKey)
	if err != nil {
		return "", fmt.Errorf("failed to compute request hash")
	}
	return cacheKeyHash, nil
}

// computeRequestID results in a value that uniquely identifies a request
// received by the proxy. It does so by SHA256 hashing the serialized request
// object containing the request path, query parameters and body parameters.
func computeRequestID(key *CacheKey) (string, error) {
	var b bytes.Buffer

	if key == nil || key.Request == nil {
		return "", fmt.Errorf("cache key is nil")
	}

	cloned := key.Request.Clone(context.Background())
	cloned.Header.Del(VaultIndexHeaderName)
	cloned.Header.Del(VaultForwardHeaderName)
	cloned.Header.Del(VaultInconsistentHeaderName)
	cloned.Header.Del(VaultCacheControlHeaderName)
	// Serialize the request
	if err := cloned.Write(&b); err != nil {
		return "", fmt.Errorf("failed to serialize request: %v", err)
	}

	// Reset the request body after it has been closed by Write
	key.Request.Body = ioutil.NopCloser(bytes.NewReader(key.RequestBody))

	// Append key.Token into the byte slice. Just in case the token was only
	// passed directly in CacheKey.Token, and not in a header.
	if _, err := b.Write([]byte(key.Token)); err != nil {
		return "", fmt.Errorf("failed to write token to hash input: %w", err)
	}

	return hex.EncodeToString(cryptoutil.Blake2b256Hash(b.String())), nil
}

func (c *Cache) Set(keyStr string, data *CacheData) {
	c.data.Set(keyStr, data, gocache.DefaultExpiration)
}

func (c *Cache) Get(keyStr string) (data *CacheData, err error) {
	dataRaw, found := c.data.Get(keyStr)
	if found && dataRaw != nil {
		var ok bool
		data, ok = dataRaw.(*CacheData)
		if !ok {
			return nil, fmt.Errorf("failed to convert cache item to CacheData for key %v", keyStr)
		}
	}

	return data, nil
}

func (c *Cache) Remove(keyStr string) {
	c.data.Delete(keyStr)
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

func setCacheOptions(cacheControlHeader string, options *CacheOptions) {
	values := strings.Split(cacheControlHeader, ",")
	options.cacheable = strutil.StrListContains(values, headerOptionCacheable)
	options.recache = strutil.StrListContains(values, headerOptionRecache)
}

func shallFetchCache(r *http.Request, cache *Cache) bool {
	if cache == nil {
		return false
	}
	cacheControl := r.Header.Get(VaultCacheControlHeaderName)
	options := &CacheOptions{}
	setCacheOptions(cacheControl, options)
	cacheable := options.cacheable && !options.recache
	return r.Method == "GET" && cacheable
}

func shallRefreshCache(r *http.Request, cache *Cache) bool {
	if cache == nil {
		return false
	}
	cacheControl := r.Header.Get(VaultCacheControlHeaderName)
	options := &CacheOptions{}
	setCacheOptions(cacheControl, options)
	cacheable := options.cacheable || options.recache
	return r.Method == "GET" && cacheable
}

func fetchFromCache(w http.ResponseWriter, data *CacheData) {
	copyHeaders(w.Header(), data.Header)
	w.WriteHeader(data.StatusCode)
	w.Write(data.Body)
}

func retrieveData(resp *http.Response) *CacheData {
	data := &CacheData{}
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
