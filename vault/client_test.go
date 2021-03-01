package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	secretFunc  func() *api.Secret
	with1sLease = &api.Secret{
		Auth: &api.SecretAuth{
			LeaseDuration: 1,
			ClientToken:   "foo",
			Renewable:     true,
		},
	}
	with1hLease = &api.Secret{
		Auth: &api.SecretAuth{
			LeaseDuration: 3600,
			ClientToken:   "foo",
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
	vaultCh := make(chan *http.Request)
	vault := fakeVault(vaultCh)
	defer vault.Close()
	stsServer := fakeSTS()
	defer stsServer.Close()

	t.Run("TestRenewToken_RespectsCancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, renewToken(ctx, nil, nil, nil))
	})

	t.Run("TestRenewToken_SingleRenewalHappyPath", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		logBuffer := bytes.Buffer{}
		logger := log.New(&logBuffer, "", 0)
		client, err := api.NewClient(&api.Config{
			Address: vault.URL,
		})
		require.NoError(t, err)

		secretFunc = generateSecretFunc(t, []*api.Secret{
			with1sLease,
			with1hLease,
		})
		var vaultCalls []*http.Request
		go func() {
			// Wait for two calls to renew to ensure the renewer in the background
			// thread has completed one whole renewal cycle.
			r := <-vaultCh
			vaultCalls = append(vaultCalls, r)
			r = <-vaultCh
			vaultCalls = append(vaultCalls, r)
			cancel()
		}()
		require.NoError(t, renewToken(ctx, logger, client, with1sLease))
		require.Contains(t, vaultCalls[0].URL.Path, "auth/token/renew-self")
		require.Contains(t, vaultCalls[1].URL.Path, "auth/token/renew-self")
		require.Contains(t, string(logBuffer.Bytes()), "successfully renewed token")
	})

	t.Run("TestRefreshToken_AttemptsReAuthIfNonRenewable", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		logBuffer := bytes.Buffer{}
		logger := log.New(&logBuffer, "", 0)
		client, err := api.NewClient(&api.Config{
			Address: vault.URL,
		})
		require.NoError(t, err)
		config := config.AuthConfig{
			Provider: "aws",
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

		secretFunc = generateSecretFunc(t, []*api.Secret{
			with1sLease,
			with1hLease,
		})
		var vaultCalls []*http.Request
		go func() {
			r := <-vaultCh
			vaultCalls = append(vaultCalls, r)
			r = <-vaultCh
			vaultCalls = append(vaultCalls, r)
			cancel()
		}()
		refreshToken(ctx, logger, stsSvc, client, config, nonRenewable)
		require.Contains(t, vaultCalls[0].URL.Path, "auth/aws/login")
		require.Contains(t, vaultCalls[1].URL.Path, "auth/token/renew-self")
		logs := string(logBuffer.Bytes())
		require.Contains(t, logs, fmt.Sprintf("renewer stopped: %s", api.ErrRenewerNotRenewable))
		require.Contains(t, logs, "attempting to re-authenticate to Vault")
		require.NotContains(t, logs, "failed login")
		require.NotContains(t, logs, "backing off re-auth")
		require.Contains(t, logs, "renewer finished after reaching maximum lease")
	})
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		previous time.Duration
		max      time.Duration
		expMin   time.Duration
		expMax   time.Duration
	}{
		{
			1000 * time.Millisecond,
			60000 * time.Millisecond,
			1500 * time.Millisecond,
			2000 * time.Millisecond,
		},
		{
			1000 * time.Millisecond,
			5000 * time.Millisecond,
			1500 * time.Millisecond,
			2000 * time.Millisecond,
		},
		{
			4000 * time.Millisecond,
			5000 * time.Millisecond,
			3750 * time.Millisecond,
			5000 * time.Millisecond,
		},
	}

	for _, test := range tests {
		for i := 0; i < 100; i++ {
			backoff := calculateBackoff(test.previous, test.max)

			// Verify that the new backoff is 75-100% of 2*previous, but <= than the max
			if backoff < test.expMin || backoff > test.expMax {
				t.Fatalf("expected backoff in range %v to %v, got: %v", test.expMin, test.expMax, backoff)
			}
		}
	}
}

func fakeVault(vaultCh chan *http.Request) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			vaultCh <- r
		}()
		bytes, err := json.Marshal(secretFunc())
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

func generateSecretFunc(t *testing.T, secrets []*api.Secret) func() *api.Secret {
	t.Helper()
	return func() *api.Secret {
		t.Helper()
		require.NotEmpty(t, secrets)
		secret := secrets[0]
		secrets = secrets[1:]
		return secret
	}
}

func fakeSTS() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
}
