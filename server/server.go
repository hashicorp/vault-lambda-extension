package server

import (
	"errors"
	"fmt"
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
		cl, err := vaultClient.Clone()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		cl.SetToken(vaultClient.Token())

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		logger.Printf("Proxying %s %s\n", r.Method, r.URL.Path)
		resp, err := proxyRequest(cl, r, body)
		if err != nil {
			if resp != nil && resp.Error() != nil {
				// If we got an api.Response error, we'll just return that below without modification.
			} else {
				http.Error(w, err.Error(), 502)
				return
			}
		}

		w.WriteHeader(resp.StatusCode)
		headers := w.Header()
		for k, vv := range resp.Header {
			for _, v := range vv {
				headers.Add(k, v)
			}
		}
		defer resp.Body.Close()
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			http.Error(w, "failed to write response back to requester", 500)
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

	resp, err := client.RawRequestWithContext(r.Context(), fwReq)
	if err != nil {
		errString := fmt.Sprintf("failed to proxy request: %s", err)
		if resp != nil {
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			errString += fmt.Sprintf("\n\n(%d) %s", resp.StatusCode, string(body))
			if err != nil {
				errString += fmt.Sprintf("\n\nfailed to read response body: %s", err)
			}
		}
		return nil, errors.New(errString)
	}
	return resp, nil
}
