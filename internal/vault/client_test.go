// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault-lambda-extension/internal/ststest"
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
			LeaseDuration: 36000,
			ClientToken:   "foo-10h-token",
			Renewable:     true,
		},
	}
)

func TestTokenRenewal(t *testing.T) {
	vault := fakeVault()
	defer vault.Close()
	ses := session.Must(session.NewSession())
	stsServer := ststest.FakeSTS(ses)
	defer stsServer.Close()

	generateVaultClient := func() *api.Client {
		vaultClient, err := api.NewClient(&api.Config{
			Address: vault.URL,
		})
		require.NoError(t, err)
		return vaultClient
	}
	stsSvc := sts.New(ses)

	t.Run("TestExpired", func(t *testing.T) {
		now := time.Now()
		for _, tc := range []struct {
			name        string
			expiry      time.Time
			gracePeriod time.Duration
			expired     bool
		}{
			{
				name:    "defaults to expired",
				expired: true,
			},
			{
				name:        "not expired",
				expiry:      now.Add(time.Hour),
				gracePeriod: (10 * time.Second),
				expired:     false,
			},
			{
				name:        "expired: falls inside grace period",
				expiry:      now.Add(time.Hour),
				gracePeriod: time.Hour,
				expired:     true,
			},
			{
				name:        "expired: expiry time in the past",
				expiry:      now.Add(-time.Hour),
				gracePeriod: time.Second,
				expired:     true,
			},
		} {
			c := Client{
				tokenExpiry:            tc.expiry,
				tokenExpiryGracePeriod: tc.gracePeriod,
			}
			require.Equal(t, tc.expired, c.expired())
		}
	})

	t.Run("TestShouldRenew", func(t *testing.T) {
		now := time.Now()
		for _, tc := range []struct {
			name      string
			expiry    time.Time
			ttl       time.Duration
			renewable bool
			expected  bool
		}{
			{
				name:      "should renew",
				expiry:    now.Add(time.Minute),
				ttl:       time.Hour,
				renewable: true,
				expected:  true,
			},
			{
				name:      "non-renewable token",
				expiry:    now.Add(time.Minute),
				ttl:       time.Hour,
				renewable: false,
				expected:  false,
			},
			{
				name:      "lots of TTL still remaining",
				expiry:    now.Add(time.Hour),
				ttl:       time.Hour,
				renewable: true,
				expected:  false,
			},
		} {
			c := Client{
				tokenExpiry:    tc.expiry,
				tokenTTL:       tc.ttl,
				tokenRenewable: tc.renewable,
			}
			require.Equal(t, tc.expected, c.shouldRenew())
		}
	})

	t.Run("TestToken_AlreadyLoggedIn_NoVaultCalls", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		c := Client{
			VaultClient: generateVaultClient(),
			logger:      hclog.Default(),

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
			logger:      hclog.Default(),
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
		require.Equal(t, time.Hour, c.tokenTTL)
		require.True(t, c.tokenRenewable)
		require.True(t, c.tokenExpiry.After(time.Now().Add(55*time.Minute)))
	})

	t.Run("TestToken_MakesLoginCallIfExpired_PropagatesError", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		c := Client{
			VaultClient: generateVaultClient(),
			logger:      hclog.Default(),
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
			logger:      hclog.Default(),
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

		// Token expiry should now be in another 10 hours
		require.True(t, c.tokenExpiry.After(time.Now().Add(9*time.Hour)))
		require.Equal(t, 10*time.Hour, c.tokenTTL)
		require.True(t, c.tokenRenewable)
	})

	t.Run("TestToken_MakesRenewCallAt90%TTL_ErrorIsLoggedInsteadOfReturned", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		vaultClient := generateVaultClient()
		vaultClient.SetToken(t.Name())
		c := Client{
			VaultClient: vaultClient,
			logger:      hclog.Default(),
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
	})

	t.Run("TestToken_NoRenewRequestIfNotRenewable", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		vaultClient := generateVaultClient()
		vaultClient.SetToken(t.Name())
		c := Client{
			VaultClient: vaultClient,
			logger:      hclog.Default(),
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

func TestParseTokenExpiryGracePeriod(t *testing.T) {
	for _, tc := range []struct {
		duration string
		expected time.Duration
	}{
		{"", 10 * time.Second},
		{"10000000000ns", 10 * time.Second},
		{"1ns", time.Nanosecond},
		{"2h", 2 * time.Hour},
	} {
		require.NoError(t, os.Setenv(tokenExpiryGracePeriodEnv, tc.duration))
		actual, err := parseTokenExpiryGracePeriod()
		require.NoError(t, err)
		require.Equal(t, tc.expected, actual)
	}

	// Error case.
	require.NoError(t, os.Setenv(tokenExpiryGracePeriodEnv, "foo"))
	_, err := parseTokenExpiryGracePeriod()
	require.Error(t, err)
}

const userAgent = "abcd"

func TestUserAgentHeaderAddition(t *testing.T) {
	vault := fakeVault()
	generateVaultClient := func() *api.Client {
		vaultClient, err := api.NewClient(&api.Config{
			Address: vault.URL,
		})
		require.NoError(t, err)
		return vaultClient
	}

	t.Run("Ensure request contains header if decorator set", func(t *testing.T) {
		vaultRequests = []*http.Request{}
		vaultClient := generateVaultClient()
		vaultClient.SetToken(t.Name())
		c := Client{
			VaultClient: vaultClient,
			logger:      hclog.Default(),
			//stsSvc:      stsSvc,

			tokenRenewable: true,
			tokenExpiry:    time.Now().Add(time.Hour),
			tokenTTL:       10 * time.Hour,
		}
		secretFunc = generateSecretFunc(t, []*api.Secret{
			with10hLease,
		})
		c.VaultClient = c.VaultClient.WithRequestCallbacks(UserAgentRequestCallback(fakeUserAgent))

		_, err := c.Token(context.Background())
		require.NoError(t, err)

		// validate request was set and the user agent is what we expect
		require.Equal(t, 1, len(vaultRequests))
		require.Equal(t, userAgent, vaultRequests[0].Header.Get("User-Agent"))
	})
}

func fakeUserAgent(_ *api.Request) string {
	return userAgent
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
