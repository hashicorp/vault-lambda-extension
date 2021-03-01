package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/vault-lambda-extension/config"
	"github.com/hashicorp/vault/api"
)

// NewClient uses the AWS IAM auth method configured in a Vault cluster to
// authenticate the execution role and create a Vault API client.
func NewClient(ctx context.Context, logger *log.Logger, authConfig config.AuthConfig) (*api.Client, *api.Config, error) {
	config := api.DefaultConfig()
	if config.Error != nil {
		return nil, nil, fmt.Errorf("error making default vault config for extension: %w", config.Error)
	}
	vaultClient, err := api.NewClient(config)
	if err != nil {
		return nil, nil, fmt.Errorf("error making extension: %w", err)
	}

	ses := session.Must(session.NewSession())
	stsSvc := sts.New(ses)

	logger.Println("attempting Vault login")
	secret, err := login(ctx, stsSvc, vaultClient, authConfig)
	if err != nil {
		return nil, nil, err
	}

	// Start background threads to renew and re-auth as required.
	go refreshToken(ctx, logger, stsSvc, vaultClient, authConfig, secret)

	return vaultClient, config, nil
}

// login authenticates to Vault using IAM auth, and sets the client's token.
func login(ctx context.Context, stsSvc *sts.STS, client *api.Client, authConfig config.AuthConfig) (*api.Secret, error) {
	// ignore out
	req, _ := stsSvc.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{})
	req.SetContext(ctx)

	if authConfig.IAMServerID != "" {
		req.HTTPRequest.Header.Add("X-Vault-AWS-IAM-Server-ID", authConfig.IAMServerID)
	}

	if signErr := req.Sign(); signErr != nil {
		return nil, signErr
	}

	headers, err := json.Marshal(req.HTTPRequest.Header)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(req.HTTPRequest.Body)
	if err != nil {
		return nil, err
	}

	d := make(map[string]interface{})
	d["iam_http_request_method"] = req.HTTPRequest.Method
	d["iam_request_url"] = base64.StdEncoding.EncodeToString([]byte(req.HTTPRequest.URL.String()))
	d["iam_request_headers"] = base64.StdEncoding.EncodeToString(headers)
	d["iam_request_body"] = base64.StdEncoding.EncodeToString(body)
	d["role"] = authConfig.Role

	secret, err := client.Logical().Write(fmt.Sprintf("auth/%s/login", authConfig.Provider), d)
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, fmt.Errorf("got no response from the %s authentication provider", authConfig.Provider)
	}

	token, err := secret.TokenID()
	if err != nil {
		return nil, fmt.Errorf("error reading token: %s", err)
	}
	client.SetToken(token)

	return secret, nil
}

// refreshToken will handle renewing the existing token until it reaches its maximum
// lease and will continue re-authenticating and renewing until the context is cancelled.
// Should be called immediately after a token is received.
func refreshToken(ctx context.Context, logger *log.Logger, stsSvc *sts.STS, client *api.Client, config config.AuthConfig, secret *api.Secret) {
	for {
		// Sleep for the first 2/3 of the initial token TTL, as the renewer below
		// will immediately start with a renew call, and in many cases we expect
		// the Lambda function will exit before the first renewal is even needed.
		err := sleepUntilRenewingShouldStart(ctx, secret)
		if err != nil {
			logger.Printf("error while waiting for first token renewal: %s\n", err)
		}

		err = renewToken(ctx, logger, client, secret)
		if err != nil {
			logger.Printf("renewer stopped: %s\n", err)
		} else {
			logger.Println("renewer finished after reaching maximum lease")
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		sleepDuration := 100 * time.Millisecond
		for {
			logger.Println("attempting to re-authenticate to Vault")
			secret, err = login(ctx, stsSvc, client, config)
			if err != nil {
				logger.Printf("failed login: %s\n", err)
			} else {
				break
			}

			logger.Printf("backing off re-auth for %v\n", sleepDuration)
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepDuration):
			}

			sleepDuration = calculateBackoff(sleepDuration, 5*time.Minute)
		}
	}
}

func sleepUntilRenewingShouldStart(ctx context.Context, secret *api.Secret) error {
	ttl, err := secret.TokenTTL()
	if err != nil {
		return err
	}
	initialSleepDuration := time.Duration(float64(ttl.Nanoseconds()) * 0.67)
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(initialSleepDuration):
	}

	return nil
}

// renewToken will renew a token for as long as it can and then return.
// Returned error will be nil if the token ran to its maximum lease,
// or if the context/rewewal was cancelled, and non-nil otherwise.
func renewToken(ctx context.Context, logger *log.Logger, client *api.Client, secret *api.Secret) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	increment, err := secret.TokenTTL()
	if err != nil {
		return err
	}
	renewer, err := client.NewRenewer(&api.RenewerInput{
		Secret:    secret,
		Increment: int(increment.Seconds()),
	})
	go renewer.Renew()
	defer renewer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-renewer.DoneCh():
			return err

		case <-renewer.RenewCh():
			logger.Println("successfully renewed token")
		}
	}
}

// calculateBackoff determines a new backoff duration that is roughly twice
// the previous value, capped to a max value, with a measure of randomness.
// Copied directly from vault agent:
// https://github.com/hashicorp/vault/blob/8db00401a44a6448d3622c97a8f1676405153deb/command/agent/auth/auth.go#L329-L340
func calculateBackoff(previous, max time.Duration) time.Duration {
	maxBackoff := 2 * previous
	if maxBackoff > max {
		maxBackoff = max
	}

	// Trim a random amount (0-25%) off the doubled duration
	trim := rand.Int63n(int64(maxBackoff) / 4)
	return maxBackoff - time.Duration(trim)
}
