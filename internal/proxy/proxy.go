package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/hashicorp/vault-lambda-extension/internal/vault"
	"github.com/hashicorp/vault/sdk/helper/consts"
)

const (
	VaultIndexHeaderName        = "X-Vault-Index"
	VaultInconsistentHeaderName = "X-Vault-Inconsistent"
	VaultForwardHeaderName      = "X-Vault-Forward"
)

// New returns an unstarted HTTP server with health and proxy handlers.
func New(logger *log.Logger, client *vault.Client) *http.Server {
	cache := setupCache()
	mux := http.ServeMux{}
	mux.HandleFunc("/", proxyHandler(logger, client, cache))
	srv := http.Server{
		Handler: &mux,
	}

	return &srv
}

// The proxyHandler borrows from the Send function in Vault Agent's proxy:
// https://github.com/hashicorp/vault/blob/22b486b651b8956d32fb24e77cef4050df7094b6/command/agent/cache/api_proxy.go
func proxyHandler(logger *log.Logger, client *vault.Client, cache *Cache) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := client.Token(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get valid Vault token: %s", err), http.StatusInternalServerError)
			return
		}
		logger.Printf("Proxying %s %s\n", r.Method, r.URL.Path)
		fwReq, err := proxyRequest(r, client.VaultConfig.Address, token)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to generate proxy request: %s", err), http.StatusInternalServerError)
			return
		}

		cacheKeyHash := ""
		if shallFetchCache(fwReq, cache) {
			// Construct the hash for this request to use as the cache key
			cacheKeyHash, err = makeRequestHash(logger, r, token)
			if err != nil {
				logger.Printf("failed to compute request hash: %s", err)
				http.Error(w, "failed to read request", http.StatusInternalServerError)
				return
			}
			// Check the cache for this request
			data, err := cache.Get(cacheKeyHash)
			if err != nil {
				logger.Printf("failed to fetch from cache: %s", err)
			}
			if data != nil {
				logger.Printf("Cache hit for: %s %s", r.Method, r.URL.Path)
				fetchFromCache(w, data)
				return
			}
		}

		resp, err := client.VaultConfig.HttpClient.Do(fwReq)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to proxy request: %s", err), http.StatusBadGateway)
			return
		}

		defer resp.Body.Close()

		if shallRefreshCache(r, cache) {
			if cacheKeyHash == "" {
				// Construct the hash for this request to use as the cache key
				cacheKeyHash, err = makeRequestHash(logger, r, token)
				if err != nil {
					logger.Printf("failed to compute request hash: %s", err)
					http.Error(w, "failed to read request", http.StatusInternalServerError)
					return
				}
			}

			data := retrieveData(resp)

			cache.Set(cacheKeyHash, data)

			logger.Printf("Refreshed cache for: %s %s", r.Method, r.URL.Path)

			fetchFromCache(w, data)

		} else {
			copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)

			_, err = io.Copy(w, resp.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to write response back to requester: %s", err), http.StatusInternalServerError)
				return
			}
		}

		logger.Printf("Successfully proxied %s %s\n", r.Method, r.URL.Path)
	}
}

func proxyRequest(r *http.Request, vaultAddress string, token string) (*http.Request, error) {
	// http.Transport will transparently request gzip and decompress the response, but only if
	// the client doesn't manually set the header. Removing any Accept-Encoding header allows the
	// transparent compression to occur.
	r.Header.Del("Accept-Encoding")

	vault, err := url.Parse(vaultAddress)
	if err != nil {
		return nil, err
	}
	upstream := *r.URL
	upstream.Scheme = vault.Scheme
	upstream.Host = vault.Host

	fwReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstream.String(), r.Body)
	if err != nil {
		return nil, err
	}
	fwReq.Header = r.Header
	fwReq.Header.Add(consts.AuthHeaderName, token)

	return fwReq, nil
}
