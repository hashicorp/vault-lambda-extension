package vault

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/vault/api"
)

// NewClient uses the AWS IAM auth method configured in a Vault cluster to
// authenticate the execution role and create a Vault API client.
func NewClient(logger *log.Logger, vaultAuthRole, vaultAuthProvider string, vaultIAMServerID string) (*api.Client, *api.Config, error) {
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

	// ignore out
	req, _ := stsSvc.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{})

	if vaultIAMServerID != "" {
		req.HTTPRequest.Header.Add("X-Vault-AWS-IAM-Server-ID", vaultIAMServerID)
	}

	if signErr := req.Sign(); signErr != nil {
		return nil, nil, signErr
	}

	headers, err := json.Marshal(req.HTTPRequest.Header)
	if err != nil {
		return nil, nil, err
	}

	body, err := ioutil.ReadAll(req.HTTPRequest.Body)
	if err != nil {
		return nil, nil, err
	}

	d := make(map[string]interface{})
	d["iam_http_request_method"] = req.HTTPRequest.Method
	d["iam_request_url"] = base64.StdEncoding.EncodeToString([]byte(req.HTTPRequest.URL.String()))
	d["iam_request_headers"] = base64.StdEncoding.EncodeToString(headers)
	d["iam_request_body"] = base64.StdEncoding.EncodeToString(body)
	d["role"] = vaultAuthRole

	logger.Println("attemping Vault login...")
	resp, err := vaultClient.Logical().Write(fmt.Sprintf("auth/%s/login", vaultAuthProvider), d)
	if err != nil {
		return nil, nil, err
	}
	if resp == nil {
		return nil, nil, fmt.Errorf("got no response from the %s authentication provider", vaultAuthProvider)
	}

	token, err := parseToken(resp)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing token: %s", err)
	}
	vaultClient.SetToken(token)

	return vaultClient, config, nil
}

func parseToken(resp *api.Secret) (string, error) {
	var err error
	token, err := resp.TokenID()
	if err != nil {
		return "", err
	}

	return token, nil
}
