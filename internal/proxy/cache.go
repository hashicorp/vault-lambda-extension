// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxy

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/cryptoutil"
	"github.com/hashicorp/vault/sdk/helper/locksutil"
	"github.com/hashicorp/vault/sdk/helper/strutil"
	gocache "github.com/patrickmn/go-cache"
)

const (
	// Return a cached response if it exists, otherwise fall back to Vault and
	// cache the response
	headerOptionCacheable = "cache"

	// Send the request to Vault and cache the response
	headerOptionRecache = "recache"

	// Ignore the cache and send the request to Vault, do not cache the response
	headerOptionNocache = "nocache"
)

type Cache struct {
	data *gocache.Cache

	// defaultOn means caching is enabled for all requests without the need for
	// explicitly setting a caching header
	defaultOn bool

	// requestLocks is used during cache lookup to ensure that identical
	// requests made in parallel do not all hit vault
	requestLocks []*locksutil.LockEntry
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
	nocache   bool
}

func NewCache(cc config.CacheConfig) *Cache {
	return &Cache{
		data:         gocache.New(cc.TTL, cc.TTL),
		defaultOn:    cc.DefaultEnabled,
		requestLocks: locksutil.CreateLocks(),
	}
}

// constructs the CacheKey for this request and token and returns the SHA256
// hash
func makeRequestHash(logger hclog.Logger, r *http.Request, token string) (string, error) {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				logger.Error("error closing request body", "error", err)
			}
		}
		return "", fmt.Errorf("failed to read request body: %w", err)
	}
	if r.Body != nil {
		if err := r.Body.Close(); err != nil {
			logger.Error("error closing request body", "error", err)
		}
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
	cloned.Header.Del(api.HeaderIndex)
	cloned.Header.Del(api.HeaderForward)
	cloned.Header.Del(api.HeaderInconsistent)
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

func setupCache(cacheConfig config.CacheConfig) *Cache {
	if cacheConfig.TTL <= 0 {
		return nil
	}
	return NewCache(cacheConfig)
}

func parseCacheOptions(cacheControlHeaders []string) *CacheOptions {
	values := []string{}
	for _, header := range cacheControlHeaders {
		values = append(values, strings.Split(header, ",")...)
	}
	options := &CacheOptions{
		cacheable: strutil.StrListContains(values, headerOptionCacheable),
		recache:   strutil.StrListContains(values, headerOptionRecache),
		nocache:   strutil.StrListContains(values, headerOptionNocache),
	}

	return options
}

func shallFetchCache(r *http.Request, cache *Cache) bool {
	if cache == nil {
		return false
	}
	options := parseCacheOptions(r.Header.Values(VaultCacheControlHeaderName))
	cacheable := (cache.defaultOn || options.cacheable) && !options.recache && !options.nocache
	return r.Method == http.MethodGet && cacheable
}

func shallRefreshCache(r *http.Request, cache *Cache) bool {
	if cache == nil {
		return false
	}
	options := parseCacheOptions(r.Header.Values(VaultCacheControlHeaderName))
	cacheable := (cache.defaultOn || options.cacheable || options.recache) && !options.nocache
	return r.Method == http.MethodGet && cacheable
}

func fetchFromCache(w http.ResponseWriter, data *CacheData) {
	copyHeaders(w.Header(), data.Header)
	w.WriteHeader(data.StatusCode)
	w.Write(data.Body)
}

func retrieveData(resp *http.Response, body []byte) *CacheData {
	return &CacheData{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}
}
