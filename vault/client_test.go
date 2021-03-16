package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/vault-lambda-extension/config"
	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

var (
	vaultRequests []*http.Request
	secretFunc    func() (*api.Secret, error)

	with1hLease = &api.Secret{
		Auth: &api.SecretAuth{
			LeaseDuration: 3600,
			ClientToken:   "foo-1h-token",
			Renewable:     true,
		},
	}
	with10hLease = &api.Secret{
		Auth: &api.SecretAuth{
			LeaseDuration: 1,
			ClientToken:   "foo-10h-token",
			Renewable:     true,
		},
	}
	nonRenewable = &api.Secret{
		Auth: &api.SecretAuth{
			Renewable: false,
		},
	}
)

func TestTokenRenewal(t *testing.T) {
	vault := fakeVault()
	defer vault.Close()
	stsServer := fakeSTS()
	defer stsServer.Close()

	nullLogger := log.New(ioutil.Discard, "", 0)
	generateVaultClient := func() *api.Client {
		vaultClient, err := api.NewClient(&api.Config{
			Address: vault.URL,
		})
		require.NoError(t, err)
		return vaultClient
	}
	ses := session.Must(session.NewSession())
	ses.Config.
		WithEndpoint(stsServer.URL).
		WithRegion("us-east-1").
		WithCredentials(credentials.NewStaticCredentialsFromCreds(credentials.Value{
			ProviderName:    session.EnvProviderName,
			AccessKeyID:     "foo",
			SecretAccessKey: "foo",
			SessionToken:    "foo",
		}))
	stsSvc := sts.New(ses)

	t.Run("TestExpiredByDefault", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		c := Client{}
		require.True(t, c.expired())
		require.False(t, c.shouldRenew())
	})

	t.Run("TestToken_AlreadyLoggedIn_NoVaultCalls", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		c := Client{
			VaultClient: generateVaultClient(),
			logger:      nullLogger,

			tokenExpiry: time.Now().Add(time.Hour),
		}
		secretFunc = nil
		_, err := c.Token(context.Background())
		require.NoError(t, err)
		require.Equal(t, 0, len(vaultRequests))
	})

	t.Run("TestToken_MakesLoginCallIfExpired", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		c := Client{
			VaultClient: generateVaultClient(),
			logger:      nullLogger,
			stsSvc:      stsSvc,
			authConfig: config.AuthConfig{
				Provider: "aws",
			},
		}
		secretFunc = generateSecretFunc(t, []*api.Secret{
			with1hLease,
		})
		token, err := c.Token(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, len(vaultRequests))
		require.Equal(t, "/v1/auth/aws/login", vaultRequests[0].URL.Path)
		require.Equal(t, "foo-1h-token", token)
	})

	t.Run("TestToken_MakesLoginCallIfExpired_PropagatesError", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		c := Client{
			VaultClient: generateVaultClient(),
			logger:      nullLogger,
			stsSvc:      stsSvc,
			authConfig: config.AuthConfig{
				Provider: "aws",
			},
		}
		secretFunc = func() (*api.Secret, error) {
			return nil, errors.New("failed login")
		}
		_, err := c.Token(context.Background())
		require.Error(t, err)
		require.Equal(t, 1, len(vaultRequests))
		require.Equal(t, "/v1/auth/aws/login", vaultRequests[0].URL.Path)
	})

	t.Run("TestToken_MakesRenewCallAt90%TTL", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		vaultClient := generateVaultClient()
		vaultClient.SetToken(t.Name())
		c := Client{
			VaultClient: vaultClient,
			logger:      nullLogger,
			stsSvc:      stsSvc,

			tokenRenewable: true,
			tokenExpiry:    time.Now().Add(time.Hour),
			tokenTTL:       10 * time.Hour,
		}
		secretFunc = generateSecretFunc(t, []*api.Secret{
			with10hLease,
		})

		token, err := c.Token(context.Background())
		require.NoError(t, err)

		// Token should not get updated by renew request.
		require.Equal(t, t.Name(), token)
		require.Equal(t, 1, len(vaultRequests))
		require.Equal(t, "/v1/auth/token/renew-self", vaultRequests[0].URL.Path)
	})

	t.Run("TestToken_MakesRenewCallAt90%TTL_ErrorIsLoggedInsteadOfReturned", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		vaultClient := generateVaultClient()
		vaultClient.SetToken(t.Name())
		loggerBuffer := bytes.Buffer{}
		c := Client{
			VaultClient: vaultClient,
			logger:      log.New(&loggerBuffer, "", 0),
			stsSvc:      stsSvc,

			tokenRenewable: true,
			tokenExpiry:    time.Now().Add(time.Hour),
			tokenTTL:       10 * time.Hour,
		}
		secretFunc = func() (*api.Secret, error) {
			return nil, errors.New("failed renew")
		}

		token, err := c.Token(context.Background())
		require.NoError(t, err)

		// Token should not get updated by failed renew request.
		require.Equal(t, t.Name(), token)
		require.Equal(t, 1, len(vaultRequests))
		require.Equal(t, "/v1/auth/token/renew-self", vaultRequests[0].URL.Path)

		require.Contains(t, loggerBuffer.String(), "failed to renew")
	})

	t.Run("TestToken_NoRenewRequestIfNotRenewable", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		vaultClient := generateVaultClient()
		vaultClient.SetToken(t.Name())
		c := Client{
			VaultClient: vaultClient,
			logger:      nullLogger,
			stsSvc:      stsSvc,

			tokenRenewable: false,
			tokenExpiry:    time.Now().Add(time.Hour),
			tokenTTL:       10 * time.Hour,
		}
		secretFunc = nil

		token, err := c.Token(context.Background())
		require.NoError(t, err)

		// Token should not get updated by renew request.
		require.Equal(t, t.Name(), token)
		require.Equal(t, 0, len(vaultRequests))
	})
}

func fakeVault() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			vaultRequests = append(vaultRequests, r)
		}()
		secret, err := secretFunc()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		bytes, err := json.Marshal(secret)
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

func generateSecretFunc(t *testing.T, secrets []*api.Secret) func() (*api.Secret, error) {
	t.Helper()
	return func() (*api.Secret, error) {
		t.Helper()
		secret := secrets[0]
		secrets = secrets[1:]
		return secret, nil
	}
}

func fakeSTS() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
}
