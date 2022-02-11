package proxy

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

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault-lambda-extension/internal/ststest"
	"github.com/hashicorp/vault-lambda-extension/internal/vault"
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
	vaultLoginResponse = &api.Secret{
		Auth: &api.SecretAuth{
			LeaseDuration: 3600,
			ClientToken:   "foo",
			Renewable:     true,
		},
	}
)

func TestProxy(t *testing.T) {
	fakeVault := fakeVault()
	defer fakeVault.Close()
	ses := session.Must(session.NewSession())
	sts := ststest.FakeSTS(ses)
	defer sts.Close()
	proxyAddr, close := startProxy(t, fakeVault.URL, ses)
	defer close()

	t.Run("happy path bare http client", func(t *testing.T) {
		fakeVaultResponse = vaultResponseFooBar
		resp, err := http.Get(fmt.Sprintf("http://%s/v1/secret/data/foo", proxyAddr))

		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
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
		fakeVaultResponse = vaultResponseFooBar
		resp, err := http.Get(fmt.Sprintf("http://%s/FailedTransport", proxyAddr))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})
}

func startProxy(t *testing.T, vaultAddress string, ses *session.Session) (string, func() error) {
	vaultConfig := api.DefaultConfig()
	require.NoError(t, vaultConfig.Error)
	vaultConfig.Address = vaultAddress
	client, err := vault.NewClient(log.New(ioutil.Discard, "", 0), vaultConfig, config.AuthConfig{}, ses)
	require.NoError(t, err)
	client.VaultConfig.Address = vaultAddress
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := New(log.New(ioutil.Discard, "", 0), client, config.CacheConfig{})
	go func() {
		_ = proxy.Serve(ln)
	}()

	return ln.Addr().String(), proxy.Close
}

func fakeVault() *httptest.Server {
	vaultResponsePtr := &fakeVaultResponse
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "login"):
			b, err := json.Marshal(vaultLoginResponse)
			if err != nil {
				http.Error(w, "failed to marshal test response", 500)
				return
			}
			_, err = w.Write(b)
			if err != nil {
				http.Error(w, "failed to write response", 500)
				return
			}
			w.WriteHeader(200)
		case strings.Contains(r.URL.Path, "FailedTransport"):
			err := hijack(w)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		case vaultResponsePtr.err != nil:
			http.Error(w, vaultResponsePtr.err.Error(), vaultResponsePtr.code)
		default:
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
		}
	}))
}

// hijack allows us to fail at the HTTP transport layer by writing an invalid
// response. The proxy should return 502 when this happens.
func hijack(w http.ResponseWriter) error {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("failed to hijack")
	}
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = buf.WriteString("Invalid HTTP response")
	if err != nil {
		return err
	}
	err = buf.Flush()
	if err != nil {
		return err
	}

	return nil
}
