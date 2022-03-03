package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault/api"
)

const (
	tokenExpiryGracePeriodEnv     = "VAULT_TOKEN_EXPIRY_GRACE_PERIOD"
	defaultTokenExpiryGracePeriod = 10 * time.Second
)

// Client holds api.Client and handles state required to renew tokens and re-auth as required.
type Client struct {
	mtx sync.Mutex

	VaultClient *api.Client
	VaultConfig *api.Config

	logger     hclog.Logger
	stsSvc     *sts.STS
	authConfig config.AuthConfig

	// Token refresh/renew data.
	tokenExpiryGracePeriod time.Duration
	tokenExpiry            time.Time
	tokenTTL               time.Duration
	tokenRenewable         bool
}

// NewClient uses the AWS IAM auth method configured in a Vault cluster to
// authenticate the execution role and create a Vault API client.
func NewClient(logger hclog.Logger, vaultConfig *api.Config, authConfig config.AuthConfig, awsSes *session.Session) (*Client, error) {
	vaultClient, err := api.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("error making extension: %w", err)
	}

	expiryGracePeriod, err := parseTokenExpiryGracePeriod()
	if err != nil {
		return nil, err
	}

	client := &Client{
		VaultClient: vaultClient,
		VaultConfig: vaultConfig,

		logger:     logger,
		stsSvc:     sts.New(awsSes),
		authConfig: authConfig,

		tokenExpiryGracePeriod: expiryGracePeriod,
	}

	return client, nil
}

// Token synchronously renews/re-auths as required and returns a Vault token.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.expired() {
		c.logger.Debug("authenticating to Vault")
		err := c.login(ctx)
		if err != nil {
			return "", err
		}
	} else if c.shouldRenew() {
		// Renew but don't retry or bail on errors, just best effort.
		c.logger.Debug("renewing Vault token")
		err := c.renew()
		if err != nil {
			c.logger.Error("failed to renew token but attempting to continue", "error", err)
		}
	}

	return c.VaultClient.Token(), nil
}

// login authenticates to Vault using IAM auth, and sets the client's token.
func (c *Client) login(ctx context.Context) error {
	// ignore out
	req, _ := c.stsSvc.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{})
	req.SetContext(ctx)

	if c.authConfig.IAMServerID != "" {
		req.HTTPRequest.Header.Add("X-Vault-AWS-IAM-Server-ID", c.authConfig.IAMServerID)
	}

	if signErr := req.Sign(); signErr != nil {
		return signErr
	}

	headers, err := json.Marshal(req.HTTPRequest.Header)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(req.HTTPRequest.Body)
	if err != nil {
		return err
	}

	d := make(map[string]interface{})
	d["iam_http_request_method"] = req.HTTPRequest.Method
	d["iam_request_url"] = base64.StdEncoding.EncodeToString([]byte(req.HTTPRequest.URL.String()))
	d["iam_request_headers"] = base64.StdEncoding.EncodeToString(headers)
	d["iam_request_body"] = base64.StdEncoding.EncodeToString(body)
	d["role"] = c.authConfig.Role

	secret, err := c.VaultClient.Logical().Write(fmt.Sprintf("auth/%s/login", c.authConfig.Provider), d)
	if err != nil {
		return err
	}
	if secret == nil {
		return fmt.Errorf("got no response from the %s authentication provider", c.authConfig.Provider)
	}

	token, err := secret.TokenID()
	if err != nil {
		return fmt.Errorf("error reading token: %s", err)
	}
	c.VaultClient.SetToken(token)

	return c.updateTokenMetadata(secret)
}

func (c *Client) renew() error {
	secret, err := c.VaultClient.Auth().Token().RenewSelf(int(c.tokenTTL.Seconds()))
	if err != nil {
		return err
	}

	return c.updateTokenMetadata(secret)
}

// Stores metadata about token lease that informs when to re-auth or renew.
func (c *Client) updateTokenMetadata(secret *api.Secret) error {
	var err error
	c.tokenTTL, err = secret.TokenTTL()
	if err != nil {
		return err
	}

	c.tokenExpiry = time.Now().Add(c.tokenTTL)
	c.tokenRenewable, err = secret.TokenIsRenewable()
	if err != nil {
		return err
	}

	return nil
}

// Returns true if current time is after tokenExpiry, or within 10s.
func (c *Client) expired() bool {
	return time.Now().Add(c.tokenExpiryGracePeriod).After(c.tokenExpiry)
}

// Returns true if tokenExpiry time is in less than 20% of tokenTTL.
func (c *Client) shouldRenew() bool {
	remaining := time.Until(c.tokenExpiry)
	return c.tokenRenewable && remaining.Nanoseconds() < c.tokenTTL.Nanoseconds()/5
}

func parseTokenExpiryGracePeriod() (time.Duration, error) {
	var err error
	expiryGracePeriod := defaultTokenExpiryGracePeriod

	expiryGracePeriodString := strings.TrimSpace(os.Getenv(tokenExpiryGracePeriodEnv))
	if expiryGracePeriodString != "" {
		expiryGracePeriod, err = time.ParseDuration(expiryGracePeriodString)
		if err != nil {
			return 0, fmt.Errorf("unable to parse %q environment variable as a valid duration: %w", tokenExpiryGracePeriodEnv, err)
		}
	}

	return expiryGracePeriod, nil
}
