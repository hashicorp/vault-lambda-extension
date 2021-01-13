package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

type vaultResponse struct {
	secret *api.Secret
	err    error
	code   int
}

var fakeVaultResponse vaultResponse

func TestProxy(t *testing.T) {
	vault := fakeVault()
	cl, err := api.NewClient(&api.Config{
		Address: vault.URL,
	})
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := New(log.New(ioutil.Discard, "[proxy server] ", 0), ln.Addr().String(), cl)
	go func() {
		_ = proxy.Serve(ln)
	}()

	proxyClient, err := api.NewClient(&api.Config{
		Address: "http://" + proxy.Addr,
	})

	t.Run("_health endpoint works", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://%s/_health", proxy.Addr))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
	})

	t.Run("happy path bare http client", func(t *testing.T) {
		fakeVaultResponse = vaultResponse{
			secret: &api.Secret{
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
		}
		resp, err := http.Get(fmt.Sprintf("http://%s/v1/secret/data/foo", proxy.Addr))
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
		fakeVaultResponse = vaultResponse{
			secret: &api.Secret{
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
		}
		resp, err := proxyClient.Logical().Read("secret/data/foo")
		require.NoError(t, err)
		require.Equal(t, "bar", resp.Data["foo"])
	})
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
