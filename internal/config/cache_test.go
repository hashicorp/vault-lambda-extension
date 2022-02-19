package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheConfig(t *testing.T) {
	t.Run("Valid vault cache TTL", func(t *testing.T) {
		defer os.Unsetenv(VaultCacheTTL)
		ttlArray := []string{"15m", "2s", "1h3m", "0h2m3s", "1h2m3s", "15s"}
		for _, ttl := range ttlArray {
			os.Setenv(VaultCacheTTL, ttl)
			cacheConfig := CacheConfigFromEnv()
			expectedTTL, err := time.ParseDuration(ttl)
			require.NoError(t, err)
			assert.Equal(t, expectedTTL, cacheConfig.TTL)
			assert.False(t, cacheConfig.DefaultEnabled)
		}
	})

	t.Run("Invalid vault cache TTL", func(t *testing.T) {
		defer os.Unsetenv(VaultCacheTTL)
		ttlArray := []string{"15sm", "2st", "1h3m5t", "-0h2m3s", "15", "-15s"}
		for _, ttl := range ttlArray {
			os.Setenv(VaultCacheTTL, ttl)
			cacheConfig := CacheConfigFromEnv()
			assert.LessOrEqual(t, cacheConfig.TTL, int64(0))
		}
	})

	t.Run("Valid vault default cache enabled", func(t *testing.T) {
		defer os.Unsetenv(VaultCacheTTL)
		defer os.Unsetenv(VaultCacheEnabled)
		enabled := []string{"true", "t", "1", "True"}
		for _, e := range enabled {
			os.Setenv(VaultCacheTTL, "5m")
			os.Setenv(VaultCacheEnabled, e)
			cacheConfig := CacheConfigFromEnv()
			assert.True(t, cacheConfig.DefaultEnabled)
		}
	})

	t.Run("False or invalid vault default cache enabled shall result in DefaultEnabled=false", func(t *testing.T) {
		defer os.Unsetenv(VaultCacheTTL)
		defer os.Unsetenv(VaultCacheEnabled)
		enabled := []string{"false", "f", "0", "False", "no", "nocache"}
		for _, e := range enabled {
			os.Setenv(VaultCacheTTL, "5m")
			os.Setenv(VaultCacheEnabled, e)
			cacheConfig := CacheConfigFromEnv()
			assert.False(t, cacheConfig.DefaultEnabled)
		}
	})
}
