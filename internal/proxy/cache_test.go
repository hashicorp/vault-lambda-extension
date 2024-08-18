// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_computeRequestID(t *testing.T) {
	tests := []struct {
		name    string
		req     *CacheKey
		want    string
		wantErr bool
	}{
		{
			"basic",
			&CacheKey{
				Request: &http.Request{
					URL: &url.URL{
						Path: "test",
					},
				},
			},
			"7b5db388f211fd9edca8c6c254831fb01ad4e6fe624dbb62711f256b5e803717",
			false,
		},
		{
			"ignore consistency headers",
			&CacheKey{
				Request: &http.Request{
					URL: &url.URL{
						Path: "test",
					},
					Header: http.Header{
						api.HeaderIndex:        []string{"foo"},
						api.HeaderInconsistent: []string{"foo"},
						api.HeaderForward:      []string{"foo"},
					},
				},
			},
			"7b5db388f211fd9edca8c6c254831fb01ad4e6fe624dbb62711f256b5e803717",
			false,
		},
		{
			"ignore distributed tracing headers",
			&CacheKey{
				Request: &http.Request{
					URL: &url.URL{
						Path: "test",
					},
					Header: http.Header{
						"Traceparent": []string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
						"Tracestate":  []string{"rojo=00f067aa0ba902b7,congo=t61rcWkgMzE"},
					},
				},
			},
			"7b5db388f211fd9edca8c6c254831fb01ad4e6fe624dbb62711f256b5e803717",
			false,
		},
		{
			"nil CacheKey",
			nil,
			"",
			true,
		},
		{
			"empty CacheKey",
			&CacheKey{},
			"",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := computeRequestID(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("actual_error: %v, expected_error: %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, string(tt.want)) {
				t.Errorf("bad: index id; actual: %q, expected: %q", got, string(tt.want))
			}
		})
	}
}

func TestCache_computeRequestID_moreTests(t *testing.T) {
	t.Run("multiple times", func(t *testing.T) {
		req := &CacheKey{
			Request: &http.Request{
				URL: &url.URL{
					Path: "test",
				},
				Header: http.Header{
					api.HeaderIndex:        []string{"foo"},
					api.HeaderInconsistent: []string{"foo"},
					api.HeaderForward:      []string{"foo"},
				},
			},
		}
		got, err := computeRequestID(req)
		require.NoError(t, err)
		got2, err := computeRequestID(req)
		require.NoError(t, err)
		assert.Equal(t, got, got2)
	})

	t.Run("token header changes hash", func(t *testing.T) {
		cacheKey := CacheKey{
			Token: "blue",
			Request: &http.Request{
				URL: &url.URL{
					Path: "test",
				},
				Header: http.Header{
					consts.AuthHeaderName: []string{"blue"},
				},
			},
			RequestBody: nil,
		}
		cacheKeyHash, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash)

		// Remove the token header
		cacheKey.Request.Header.Del(consts.AuthHeaderName)
		cacheKeyHash2, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash2)

		assert.NotEqual(t, cacheKeyHash, cacheKeyHash2)
	})

	t.Run("namespace header changes hash", func(t *testing.T) {
		cacheKey := CacheKey{
			Token: "blue",
			Request: &http.Request{
				URL: &url.URL{
					Path: "test",
				},
				Header: http.Header{
					consts.AuthHeaderName:      []string{"blue"},
					consts.NamespaceHeaderName: []string{"namespaced"},
				},
			},
			RequestBody: nil,
		}
		cacheKeyHash, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash)

		// Remove the namespace header
		cacheKey.Request.Header.Del(consts.NamespaceHeaderName)
		cacheKeyHash2, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash2)

		assert.NotEqual(t, cacheKeyHash, cacheKeyHash2)
	})

	t.Run("cache header does not change hash", func(t *testing.T) {
		cacheKey := CacheKey{
			Token: "blue",
			Request: &http.Request{
				URL: &url.URL{
					Path: "test",
				},
				Header: http.Header{
					consts.AuthHeaderName:       []string{"blue"},
					consts.NamespaceHeaderName:  []string{"namespaced"},
					VaultCacheControlHeaderName: []string{headerOptionCacheable},
				},
			},
			RequestBody: nil,
		}
		cacheKeyHash, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash)

		// Remove the cache header
		cacheKey.Request.Header.Del(VaultCacheControlHeaderName)
		cacheKeyHash2, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash2)

		assert.Equal(t, cacheKeyHash, cacheKeyHash2)
	})
}

func Test_makeRequestHash(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Path: "test",
		},
		Body: ioutil.NopCloser(bytes.NewBufferString("Hello World")),
		Header: http.Header{
			consts.AuthHeaderName: []string{"blue"},
		},
	}

	h, err := makeRequestHash(hclog.Default(), req, "blue")
	assert.NoError(t, err)
	assert.Equal(t, "b62adf8925f91450ee992596dd2fb38edb0d3270ed9edc23b98bf5f322e9ed9a", h)
}

func TestNewCache(t *testing.T) {
	cache := NewCache(config.CacheConfig{
		TTL: 10 * time.Second,
	})
	require.NotNilf(t, cache, `NewCache(%s) returns nil`, "10*time.Second")

	cacheEnabled := NewCache(config.CacheConfig{
		TTL:            10 * time.Second,
		DefaultEnabled: true,
	})
	require.NotNil(t, cacheEnabled)
}

func TestSetupCache(t *testing.T) {
	t.Run("Valid vault cache TTL shall set up and return cache successfully", func(t *testing.T) {
		ttlArray := []time.Duration{5 * time.Minute, 1 * time.Second}
		for _, ttl := range ttlArray {
			cache := setupCache(config.CacheConfig{TTL: ttl})
			require.NotNilf(t, cache, `setupCache() returns nil with env variable: %s`, ttl)
			assert.False(t, cache.defaultOn)
		}
	})

	t.Run("Invalid vault cache TTL shall fail to set up and return cache", func(t *testing.T) {
		ttlArray := []time.Duration{-2 * time.Minute, 0}
		for _, ttl := range ttlArray {
			cache := setupCache(config.CacheConfig{TTL: ttl})
			require.Nil(t, cache, `setupCache() does not return nil with ttl: %s`, ttl)
		}
	})

	t.Run("Valid vault default cache enabled shall set up and return cache successfully", func(t *testing.T) {
		cache := setupCache(config.CacheConfig{TTL: 5 * time.Minute, DefaultEnabled: true})
		require.NotNil(t, cache)
		assert.True(t, cache.defaultOn)
	})

	t.Run("False vault default cache enabled shall result in cache.enabled=false", func(t *testing.T) {
		cache := setupCache(config.CacheConfig{TTL: 5 * time.Minute, DefaultEnabled: false})
		require.NotNil(t, cache)
		assert.False(t, cache.defaultOn)
	})
}

func TestGetAfterSet(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		cacheData := &CacheData{
			Header:     nil,
			Body:       []byte(fmt.Sprint(rand.Intn(100))),
			StatusCode: http.StatusOK,
		}
		cache := NewCache(config.CacheConfig{TTL: 10 * time.Second})
		cacheKey := CacheKey{
			Token: "blue",
			Request: &http.Request{
				URL: &url.URL{
					Path: "test",
				},
				Header: http.Header{
					consts.AuthHeaderName: []string{"rose"},
				},
			},
			RequestBody: nil,
		}
		cacheKeyHash, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash)
		cache.Set(cacheKeyHash, cacheData)

		cacheDataOut, err := cache.Get(cacheKeyHash)
		require.NoError(t, err)
		assert.Equal(t, cacheData, cacheDataOut, `cache.Get() result doesn't match what was set with key: %s`, cacheKey)
	})

	t.Run("expired item not returned", func(t *testing.T) {
		cacheData := &CacheData{
			Header:     nil,
			Body:       []byte(fmt.Sprint(rand.Intn(100))),
			StatusCode: http.StatusOK,
		}
		cache := NewCache(config.CacheConfig{TTL: 10 * time.Millisecond})
		cacheKey := CacheKey{
			Token: "blue",
			Request: &http.Request{
				URL: &url.URL{
					Path: "test",
				},
				Header: http.Header{
					consts.AuthHeaderName: []string{"rose"},
				},
			},
			RequestBody: nil,
		}
		cacheKeyHash, err := computeRequestID(&cacheKey)
		require.NoError(t, err)
		require.NotEmpty(t, cacheKeyHash)
		cache.Set(cacheKeyHash, cacheData)

		time.Sleep(20 * time.Millisecond)
		cacheDataOut, err := cache.Get(cacheKeyHash)

		require.NoError(t, err)
		assert.Nil(t, cacheDataOut)
	})

	t.Run("deleted item not returned", func(t *testing.T) {
		cache := NewCache(config.CacheConfig{TTL: 1 * time.Hour})
		cacheData := &CacheData{
			Header:     nil,
			Body:       []byte(fmt.Sprint(rand.Intn(100))),
			StatusCode: http.StatusOK,
		}
		cacheKey := "test-key"
		cache.Set(cacheKey, cacheData)

		time.Sleep(5 * time.Second)
		cacheDataOut, err := cache.Get(cacheKey)
		require.NoError(t, err)
		assert.Equal(t, cacheData, cacheDataOut)

		cache.Remove(cacheKey)
		cacheDataOut2, err := cache.Get(cacheKey)
		require.NoError(t, err)
		require.Nil(t, cacheDataOut2)
	})
}

func TestShallFetchCache(t *testing.T) {
	tests := map[string]struct {
		cache        *Cache
		cacheControl string
		method       string
		expected     bool
	}{
		"Shall fetch from cache when cache-control header is 'cache'": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionCacheable,
			method:       http.MethodGet,
			expected:     true,
		},
		"Shall not fetch from cache when cache is nil": {
			cache:        nil,
			cacheControl: headerOptionCacheable,
			method:       http.MethodGet,
			expected:     false,
		},
		"Shall not fetch from cache when cache-control header is incorrect": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: "crash,cache.,cache=1,cache=true",
			method:       http.MethodGet,
			expected:     false,
		},
		"Shall not fetch from cache when cache-control is 'recache'": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionRecache,
			method:       http.MethodGet,
			expected:     false,
		},
		"Shall not fetch from cache when http method is not GET": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionCacheable,
			method:       http.MethodPost,
			expected:     false,
		},
		"Shall not fetch from cache when cache-control header is empty": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: "",
			method:       http.MethodGet,
			expected:     false,
		},
		"No cache when cache-control header is nocache": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionNocache,
			method:       http.MethodGet,
			expected:     false,
		},
		"Shall fetch from cache when default enabled": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second, DefaultEnabled: true}),
			cacheControl: "",
			method:       http.MethodGet,
			expected:     true,
		},
		"Shall not fetch from cache when default enabled and 'nocache' set": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second, DefaultEnabled: true}),
			cacheControl: headerOptionNocache,
			method:       http.MethodGet,
			expected:     false,
		},
		"Shall not fetch from cache when default enabled and 'recache' set": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second, DefaultEnabled: true}),
			cacheControl: headerOptionRecache,
			method:       http.MethodGet,
			expected:     false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/v1/uuid/s1", nil)
			r.Header.Set(VaultCacheControlHeaderName, tc.cacheControl)
			shallFetch := shallFetchCache(r, tc.cache)
			assert.Equal(t, tc.expected, shallFetch)
		})
	}
}

func TestShallFetchCache_multiple_headers(t *testing.T) {
	tests := map[string]struct {
		cacheControl []string
		expected     bool
	}{
		"No cache when 'nocache' set as second header": {
			cacheControl: []string{headerOptionCacheable, headerOptionNocache},
			expected:     false,
		},
		"No cache when 'nocache' set as first header": {
			cacheControl: []string{headerOptionNocache, headerOptionCacheable},
			expected:     false,
		},
		"Cache when repeated": {
			cacheControl: []string{headerOptionCacheable, headerOptionCacheable},
			expected:     true,
		},
		"Cache when good and bad headers": {
			cacheControl: []string{"nope", headerOptionCacheable},
			expected:     true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := NewCache(config.CacheConfig{TTL: 10 * time.Second})
			r := httptest.NewRequest(http.MethodGet, "/v1/uuid/s1", nil)
			for _, h := range tc.cacheControl {
				r.Header.Add(VaultCacheControlHeaderName, h)
			}
			shallFetch := shallFetchCache(r, cache)
			assert.Equal(t, tc.expected, shallFetch)
		})
	}
}

func TestShallRefreshCache(t *testing.T) {
	tests := map[string]struct {
		cache        *Cache
		cacheControl string
		expected     bool
	}{
		"Shall refresh cache when cache-control header is 'cache'": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionCacheable,
			expected:     true,
		},
		"Shall refresh cache when cache-control header is 'recache'": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionRecache,
			expected:     true,
		},
		"Shall refresh cache when cache-control header is 'cache,recache'": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: strings.Join([]string{headerOptionCacheable, headerOptionRecache}, ","),
			expected:     true,
		},
		"Shall not refresh cache when cache is nil": {
			cache:        nil,
			cacheControl: headerOptionCacheable,
			expected:     false,
		},
		"Shall not refresh cache when cache-control header is incorrect": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: "nope",
			expected:     false,
		},
		"Shall not refresh cache when cache-control header is empty": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: "",
			expected:     false,
		},
		"No cache when cache-control header is nocache": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second}),
			cacheControl: headerOptionNocache,
			expected:     false,
		},
		"Shall refresh cache when default enabled": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second, DefaultEnabled: true}),
			cacheControl: "",
			expected:     true,
		},
		"Shall not refresh cache when default enabled and 'nocache' set": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second, DefaultEnabled: true}),
			cacheControl: headerOptionNocache,
			expected:     false,
		},
		"Shall refresh cache when default enabled and 'recache' set": {
			cache:        NewCache(config.CacheConfig{TTL: 10 * time.Second, DefaultEnabled: true}),
			cacheControl: headerOptionRecache,
			expected:     true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/uuid/s1", nil)
			r.Header.Set(VaultCacheControlHeaderName, tc.cacheControl)
			shallFetch := shallRefreshCache(r, tc.cache)
			assert.Equal(t, tc.expected, shallFetch)
		})
	}
}

func TestShallRefreshCache_multiple_headers(t *testing.T) {
	tests := map[string]struct {
		cacheControl []string
		expected     bool
	}{
		"Shall refresh cache when cache-control headers are 'cache' and 'recache'": {
			cacheControl: []string{headerOptionCacheable, headerOptionRecache},
			expected:     true,
		},
		"Shall not refresh cache when cache-control headers are 'nocache' and 'recache'": {
			cacheControl: []string{headerOptionNocache, headerOptionRecache},
			expected:     false,
		},
		"Shall not refresh cache when cache-control headers are 'recache' and 'nocache'": {
			cacheControl: []string{headerOptionRecache, headerOptionNocache},
			expected:     false,
		},
		"Shall refresh cache with good and bad headers": {
			cacheControl: []string{headerOptionRecache, "nope", headerOptionCacheable},
			expected:     true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := NewCache(config.CacheConfig{TTL: 10 * time.Second})
			r := httptest.NewRequest("GET", "/v1/uuid/s1", nil)
			for _, h := range tc.cacheControl {
				r.Header.Add(VaultCacheControlHeaderName, h)
			}
			shallFetch := shallRefreshCache(r, cache)
			assert.Equal(t, tc.expected, shallFetch)
		})
	}
}

func TestRetrieveData(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/uuid/s1", nil)
	r.Header.Set(VaultCacheControlHeaderName, headerOptionCacheable)
	statusCode := 200
	body := "Hello World"
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          ioutil.NopCloser(bytes.NewBufferString("Not Hello World")),
		ContentLength: int64(len(body)),
		Request:       r,
		Header:        make(http.Header),
	}
	cacheData := retrieveData(resp, []byte(body))
	require.Truef(t, cacheData.StatusCode == statusCode && string(cacheData.Body) == body, `retrieveData() shall return the same body: %s`, body)
}
