package server

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/hashicorp/vault/api"
)

// New returns an unstarted HTTP server with health and proxy handlers.
func New(logger *log.Logger, vaultClient *api.Client) *http.Server {
	mux := http.ServeMux{}
	mux.HandleFunc("/", proxyHandler(logger, vaultClient))
	srv := http.Server{
		Handler: &mux,
	}

	return &srv
}

// The proxyHandler is based on the Send function from Vault Agent's proxy:
// https://github.com/hashicorp/vault/blob/22b486b651b8956d32fb24e77cef4050df7094b6/command/agent/cache/api_proxy.go
func proxyHandler(logger *log.Logger, vaultClient *api.Client) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logger.Printf("Proxying %s %s\n", r.Method, r.URL.Path)
		resp, err := proxyRequest(vaultClient, r, body)
		// resp will generally only be nil if the underlying HTTP transport errors.
		if resp == nil {
			if err == nil {
				http.Error(w, "unexpected error while proxying, both response and error are nil", http.StatusBadGateway)
			} else {
				http.Error(w, err.Error(), http.StatusBadGateway)
			}
			return
		}

		// While the underlying http client almost always only sets one of resp
		// or err, the Vault API client sets non-nil err for 4xx and 5xx codes
		// etc from Vault, but in those cases we just want to proxy the response
		// back to our client unchanged.
		headers := w.Header()
		for k, vv := range resp.Header {
			for _, v := range vv {
				headers.Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		defer resp.Body.Close()
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			http.Error(w, "failed to write response back to requester", http.StatusInternalServerError)
			return
		}
		logger.Printf("Successfully proxied %s %s\n", r.Method, r.URL.Path)
	}
}

func proxyRequest(client *api.Client, r *http.Request, body []byte) (*api.Response, error) {
	// http.Transport will transparently request gzip and decompress the response, but only if
	// the client doesn't manually set the header. Removing any Accept-Encoding header allows the
	// transparent compression to occur.
	r.Header.Del("Accept-Encoding")
	client.SetHeaders(r.Header)

	fwReq := client.NewRequest(r.Method, r.URL.Path)
	fwReq.BodyBytes = body

	query := r.URL.Query()
	if len(query) > 0 {
		fwReq.Params = query
	}

	return client.RawRequestWithContext(r.Context(), fwReq)
}
