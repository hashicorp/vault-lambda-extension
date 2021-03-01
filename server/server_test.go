package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

type vaultResponse struct {
	secret *api.Secret
	err    error
	code   int
}

var (
	fakeVaultResponse   vaultResponse
	vaultResponseFooBar = vaultResponse{
		secret: &api.Secret{
			Data: map[string]interface{}{
				"foo": "bar",
			},
		},
	}
	vaultResponse403 = vaultResponse{
		err:  errors.New("forbidden"),
		code: http.StatusForbidden,
	}
	vaultResponse500 = vaultResponse{
		err:  errors.New("internal server error"),
		code: http.StatusInternalServerError,
	}
	vaultResponse502 = vaultResponse{
		err:  errors.New("bad gateway"),
		code: http.StatusBadGateway,
	}
)

func TestProxy(t *testing.T) {
	vault := fakeVault()
	defer vault.Close()
	proxyAddr, close := startProxy(t, vault.URL)
	defer close()

	t.Run("happy path bare http client", func(t *testing.T) {
		fakeVaultResponse = vaultResponseFooBar
		resp, err := http.Get(fmt.Sprintf("http://%s/v1/secret/data/foo", proxyAddr))
		require.NoError(t, err)
		if resp.StatusCode != 200 {
			body, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			t.Fatal("Non-200 status code from proxy", string(body))
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		var secret api.Secret
		require.NoError(t, json.Unmarshal(body, &secret), string(body))
		require.Equal(t, "bar", secret.Data["foo"])
	})

	t.Run("happy path with vault client", func(t *testing.T) {
		fakeVaultResponse = vaultResponseFooBar
		proxyVaultClient, err := api.NewClient(&api.Config{
			Address: "http://" + proxyAddr,
		})
		resp, err := proxyVaultClient.Logical().Read("secret/data/foo")
		require.NoError(t, err)
		require.Equal(t, "bar", resp.Data["foo"])
	})

	t.Run("vault error codes should return unmodified", func(t *testing.T) {
		for _, tc := range []vaultResponse{vaultResponse403, vaultResponse500, vaultResponse502} {
			fakeVaultResponse = tc
			resp, err := http.Get(fmt.Sprintf("http://%s/v1/secret/data/foo", proxyAddr))
			require.NoError(t, err)
			require.Equal(t, tc.code, resp.StatusCode)
		}
	})

	t.Run("failed upstream request should give 502", func(t *testing.T) {
		// Set an invalid vault address scheme so that the HTTP request to vault fails.
		brokenProxyAddr, close := startProxy(t, strings.ReplaceAll(vault.URL, "http://", "https://"))
		defer close()
		fakeVaultResponse = vaultResponseFooBar
		resp, err := http.Get(fmt.Sprintf("http://%s/v1/secret/data/foo", brokenProxyAddr))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})

	t.Run("failure to generate proxy request should give 500", func(t *testing.T) {
		// Set an invalid vault address so that generating a proxy request fails.
		brokenProxyAddr, close := startProxy(t, "@:::")
		defer close()
		fakeVaultResponse = vaultResponseFooBar
		resp, err := http.Get(fmt.Sprintf("http://%s/v1/secret/data/foo", brokenProxyAddr))
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func startProxy(t *testing.T, vaultAddress string) (string, func() error) {
	config := api.DefaultConfig()
	require.NoError(t, config.Error)
	config.Address = vaultAddress
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tokenFunc := func() string {
		return ""
	}
	proxy := New(log.New(ioutil.Discard, "", 0), config, tokenFunc)
	go func() {
		_ = proxy.Serve(ln)
	}()

	return ln.Addr().String(), proxy.Close
}

func fakeVault() *httptest.Server {
	vaultResponsePtr := &fakeVaultResponse
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vaultResponsePtr.err != nil {
			http.Error(w, vaultResponsePtr.err.Error(), vaultResponsePtr.code)
			return
		}
		bytes, err := json.Marshal(vaultResponsePtr.secret)
		if err != nil {
			http.Error(w, "failed to marshal JSON", 500)
			return
		}
		_, err = w.Write(bytes)
		if err != nil {
			http.Error(w, "failed to write response", 500)
			return
		}
	}))
}
