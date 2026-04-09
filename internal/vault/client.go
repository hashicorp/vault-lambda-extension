// Copyright IBM Corp. 2020, 2025
// SPDX-License-Identifier: MPL-2.0

package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"

	"github.com/hashicorp/vault-lambda-extension/internal/config"
)

const (
	tokenExpiryGracePeriodEnv     = "VAULT_TOKEN_EXPIRY_GRACE_PERIOD"
	defaultTokenExpiryGracePeriod = 10 * time.Second
)

// Client holds api.Client and handles state required to renew tokens and re-auth as required.
type Client struct {
	Name    string
	Version string

	mtx sync.Mutex

	VaultClient *api.Client
	VaultConfig *api.Config

	logger     hclog.Logger
	awsCfg     aws.Config
	stsSvc     *sts.Client
	authConfig config.AuthConfig

	// Token refresh/renew data.
	tokenExpiryGracePeriod time.Duration
	tokenExpiry            time.Time
	tokenTTL               time.Duration
	tokenRenewable         bool
	tokenRevoked           bool
}

// NewClient uses the AWS IAM auth method configured in a Vault cluster to
// authenticate the execution role and create a Vault API client.
func NewClient(name, version string, logger hclog.Logger, vaultConfig *api.Config, authConfig config.AuthConfig, awsCfg aws.Config) (*Client, error) {
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
		Name:        name,
		Version:     version,

		logger:     logger,
		awsCfg:     awsCfg,
		stsSvc:     sts.NewFromConfig(awsCfg),
		authConfig: authConfig,

		tokenExpiryGracePeriod: expiryGracePeriod,
	}

	return client, nil
}

// Token synchronously renews/re-auths as required and returns a Vault token.
func (c *Client) Token(ctx context.Context) (string, error) {
	start := time.Now().Round(0)
	c.logger.Debug("fetching token")
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.expired() || c.tokenRevoked {
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

	c.logger.Debug(fmt.Sprintf("fetched token in %v", time.Since(start)))
	return c.VaultClient.Token(), nil
}

// Mark token revoked
func (c *Client) RevokeToken() {
	c.tokenRevoked = true
}

// login authenticates to Vault using IAM auth, and sets the client's token.
func (c *Client) login(ctx context.Context) error {
	authConfig := c.authConfig
	roleToAssumeArn := authConfig.AssumedRoleArn

	stsSvc := c.stsSvc

	/* If passing in a role (through VAULT_ASSUMED_ROLE_ARN enviornment variable)
	to be assumed for Vault authentication, use it instead of the function execution role */
	if roleToAssumeArn != "" {
		c.logger.Debug(fmt.Sprintf("Trying to assume role with arn of %s to authenticate with Vault", roleToAssumeArn))
		sessionName := "vault_auth"

		result, err := c.stsSvc.AssumeRole(ctx, &sts.AssumeRoleInput{
			RoleArn:         aws.String(roleToAssumeArn),
			RoleSessionName: aws.String(sessionName),
		})
		if err != nil {
			return fmt.Errorf("failed to assume role with arn of %s %w", roleToAssumeArn, err)
		}
		if result.Credentials == nil {
			return fmt.Errorf("failed to assume role with arn of %s: no credentials returned", roleToAssumeArn)
		}

		c.logger.Debug(fmt.Sprintf("Assumed role successfully with token expiration time: %s ", aws.ToTime(result.Credentials.Expiration).String()))

		assumedRoleCfg := c.awsCfg.Copy()
		if authConfig.STSEndpointRegion != "" {
			assumedRoleCfg.Region = authConfig.STSEndpointRegion
		}
		assumedRoleCfg.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
			aws.ToString(result.Credentials.AccessKeyId),
			aws.ToString(result.Credentials.SecretAccessKey),
			aws.ToString(result.Credentials.SessionToken),
		))

		stsSvc = sts.NewFromConfig(assumedRoleCfg)
	}

	presignOptions := []func(*sts.PresignOptions){}
	if c.authConfig.IAMServerID != "" {
		presignOptions = append(presignOptions, withSignedHeaderPresignOption("X-Vault-AWS-IAM-Server-ID", c.authConfig.IAMServerID))
	}

	presignedRequest, err := sts.NewPresignClient(stsSvc).PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, presignOptions...)
	if err != nil {
		return err
	}

	headers, err := json.Marshal(presignedRequest.SignedHeader)
	if err != nil {
		return err
	}

	d := make(map[string]interface{})
	d["iam_http_request_method"] = presignedRequest.Method
	d["iam_request_url"] = base64.StdEncoding.EncodeToString([]byte(presignedRequest.URL))
	d["iam_request_headers"] = base64.StdEncoding.EncodeToString(headers)
	d["iam_request_body"] = base64.StdEncoding.EncodeToString([]byte(""))
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

func withSignedHeaderPresignOption(header, value string) func(*sts.PresignOptions) {
	return sts.WithPresignClientFromClientOptions(sts.WithAPIOptions(func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("vaultIAMServerIDHeader", func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (
			out middleware.BuildOutput, metadata middleware.Metadata, err error,
		) {
			req, ok := in.Request.(*smithyhttp.Request)
			if !ok {
				return out, metadata, fmt.Errorf("unknown transport type %T", in.Request)
			}

			req.Header.Set(header, value)
			return next.HandleBuild(ctx, in)
		}), middleware.After)
	}))
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

	c.tokenRevoked = false
	c.tokenExpiry = time.Now().Round(0).Add(c.tokenTTL)
	c.tokenRenewable, err = secret.TokenIsRenewable()
	if err != nil {
		return err
	}

	return nil
}

// Returns true if current time is after tokenExpiry, or within 10s.
func (c *Client) expired() bool {
	return time.Now().Round(0).Add(c.tokenExpiryGracePeriod).After(c.tokenExpiry)
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

// UserAgentRequestCallback takes a function that returns a user agent string and will invoke that function to set
// the user agent string on the request.
func UserAgentRequestCallback(agentFunc func(request *api.Request) string) api.RequestCallback {
	return func(req *api.Request) {
		req.Headers.Set("User-Agent", agentFunc(req))
	}
}
