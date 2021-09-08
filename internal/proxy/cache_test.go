package proxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)


func TestNewCache(t *testing.T) {
	cache := NewCache(10*time.Second)
	if  cache == nil {
		t.Fatalf(`NewCache(%s) returns nil`, "10*time.Second")
	}
}

func TestSetupCache(t *testing.T) {
	t.Run("Valid vault cache TTL shall set up and return cache successfully", func(t *testing.T) {
		ttlArray := []string{"15m", "2s", "1h3m", "0h2m3s", "1h2m3s", "15s"}
		for _, ttl := range ttlArray {
			os.Setenv(vaultCacheTTL, ttl)
			cache := setupCache()
			if cache == nil {
				t.Fatalf(`setupCache() returns nil with env variable: %s`, ttl)
			}
		}
	})

	t.Run("Invalid vault cache TTL shall fail to set up and return cache", func(t *testing.T) {
		ttlArray := []string{"15sm", "2st", "1h3m5t", "-0h2m3s", "15", "-15s"}
		for _,ttl := range ttlArray{
			os.Setenv(vaultCacheTTL, ttl)
			cache := setupCache()
			if cache != nil{
				t.Fatalf(`setupCache() does not return nil with env variable: %s`, ttl)
			}
		}
	})
}

func TestGetAfterSet(t *testing.T) {
	cacheData := CacheData{Header: nil, Body: []byte(fmt.Sprint(rand.Intn(100))), StatusCode: 200}
	cache := NewCache(10*time.Second)
	cacheKey := CacheKey{Path: "/v1/secret", Version: "1"}
	cache.Set(cacheKey, cacheData)

	cacheDataOut, ok := cache.Get(cacheKey)

	if cacheDataOut.StatusCode != cacheData.StatusCode || string(cacheDataOut.Body) != string(cacheData.Body) || !ok {
		t.Fatalf(`cache.Get() does not return the same value with key: %s`, cacheKey)
	}
}

func TestShallFetchCache(t *testing.T) {
	cache := NewCache(10 * time.Second)
	t.Run("Shall fetch from cache when cacheable is 1 and recache is not 1", func(t *testing.T) {
		cacheableValue := "1"
		r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable=" + cacheableValue, nil)
		shallFetch := shallFetchCache(r, cache)
		if !shallFetch {
			t.Fatalf(`shallFetchCache() shall return true when cacheable is: %s`, cacheableValue)
		}
	})
	t.Run("Shall not fetch from cache when cache is nil", func(t *testing.T) {
		cacheableValue := "1"
		r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable=" + cacheableValue, nil)
		shallFetch := shallFetchCache(r, nil)
		if shallFetch {
			t.Fatal(`shallFetchCache() shall return false when cache is nil`)
		}
	})
	t.Run("Shall not fetch from cache when cacheable is not 1", func(t *testing.T) {
		cacheableValueArray := []string{"0", "2", "", "-", "10", "-1"}
		for _, cacheableValue := range cacheableValueArray {
			r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable="+cacheableValue, nil)
			shallFetch := shallFetchCache(r, cache)
			if shallFetch {
				t.Fatalf(`shallFetchCache() shall not return true when cacheable is: %s`, cacheableValue)
			}
		}
	})
	t.Run("Shall not fetch from cache when cacheable is 1 and recache is  1", func(t *testing.T) {
		recacheValue := "1"
		r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable=1&recache=" + recacheValue, nil)
		shallFetch := shallFetchCache(r, cache)
		if shallFetch {
			t.Fatalf(`shallFetchCache() shall return false when recache is: %s`, recacheValue)
		}
	})
	t.Run("Shall not fetch from cache when http method is not GET", func(t *testing.T) {
		httpMethod := "POST"
		r := httptest.NewRequest(httpMethod, "/v1/uuid/s1?cacheable=1", nil)
		shallFetch := shallFetchCache(r, cache)
		if shallFetch {
			t.Fatalf(`shallFetchCache() shall not return true when HTTP method is: %s`, httpMethod)
		}
	})
}


func TestShallRefreshCache(t *testing.T) {
	cache := NewCache(10 * time.Second)
	t.Run("Shall refresh cache when cacheable is 1", func(t *testing.T) {
		cacheableValue := "1"
		r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable="+cacheableValue, nil)
		shallFetch := shallRefreshCache(r, cache)
		if !shallFetch {
			t.Fatalf(`shallRefreshCache() shall return true when cacheable is: %s`, cacheableValue)
		}
	})
	t.Run("Shall not refresh cache when cacheable is nil", func(t *testing.T) {
		cacheableValue := "1"
		r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable="+cacheableValue, nil)
		shallFetch := shallRefreshCache(r, nil)
		if shallFetch {
			t.Fatal(`shallRefreshCache() shall return false when cache is nil`)
		}
	})
	t.Run("Shall not refresh cache when cacheable is not 1", func(t *testing.T) {
		cacheableValueArray := []string{"0", "2", "", "-", "10", "-1"}
		for _, cacheableValue := range cacheableValueArray {
			r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable="+cacheableValue, nil)
			shallFetch := shallRefreshCache(r, cache)
			if shallFetch {
				t.Fatalf(`shallRefreshCache() shall not return true when cacheable is: %s`, cacheableValue)
			}
		}
	})
}

func TestRetrieveData(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/uuid/s1?cacheable=1", nil)
	body := "Hello World"
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
		Request:       r,
		Header:        make(http.Header, 0),
	}
	cacheData := retrieveData(resp)
	if cacheData.StatusCode != 200 || string(cacheData.Body) != body {
		t.Fatalf(`retrieveData() shall return the same body: %s`, body)
	}
}