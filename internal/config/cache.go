package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// The time to live configuration (aka, TTL) of the cache used by proxy
	// server.
	VaultCacheTTL = "VAULT_DEFAULT_CACHE_TTL"

	// When set to `true`, every request will be saved in the cache and returned
	// from cache, making caching "opt-out" instead of "opt-in". Caching may
	// still be disabled per-request with the "nocache" cache-control header.
	VaultCacheEnabled = "VAULT_DEFAULT_CACHE_ENABLED"
)

// CacheConfig holds config for the request cache
type CacheConfig struct {
	TTL            time.Duration
	DefaultEnabled bool
}

// CacheConfigFromEnv reads config from the environment for caching
func CacheConfigFromEnv() CacheConfig {
	var cacheTTL time.Duration
	cacheTTLEnv := strings.TrimSpace(os.Getenv(VaultCacheTTL))
	if cacheTTLEnv != "" {
		var err error
		cacheTTL, err = time.ParseDuration(cacheTTLEnv)
		if err != nil {
			cacheTTL = 0
		}
	}

	defaultOn := false
	defaultOnEnv := strings.TrimSpace(os.Getenv(VaultCacheEnabled))
	if defaultOnEnv != "" {
		var err error
		defaultOn, err = strconv.ParseBool(defaultOnEnv)
		if err != nil {
			defaultOn = false
		}
	}

	return CacheConfig{
		TTL:            cacheTTL,
		DefaultEnabled: defaultOn,
	}
}
