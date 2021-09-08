package proxy

import (
	"fmt"
	"math/rand"
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

func TestSetupCacheSuccess(t *testing.T) {
	ttlArray := []string{"15m", "2s", "1h3m", "0h2m3s", "1h2m3s", "15s"}
	for _,ttl := range ttlArray{
		os.Setenv(vaultCacheTTL, ttl)
		cache := setupCache()
		if cache == nil{
		t.Fatalf(`setupCache() returns nil with env variable: %s`, ttl)
	}
	}
}

func TestSetupCacheFail(t *testing.T) {
	ttlArray := []string{"15sm", "2st", "1h3m5t", "-0h2m3s", "15", "-15s"}
	for _,ttl := range ttlArray{
		os.Setenv(vaultCacheTTL, ttl)
		cache := setupCache()
		if cache != nil{
			t.Fatalf(`setupCache() does not return nil with env variable: %s`, ttl)
		}
	}
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
