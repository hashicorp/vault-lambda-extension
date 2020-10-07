package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/vault-lambda-extension/extension"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/vault/api"
)

const (
	defaultSecretDirectory = "/tmp/vault"
	defaultSecretFile      = "secret.json"
	vaultSecretPathKey     = "VAULT_SECRET_PATH"
	vaultSecretFileKey     = "VAULT_SECRET_FILE"
	vaultSecretPathPrefix  = vaultSecretPathKey + "_"
	vaultSecretFilePrefix  = vaultSecretFileKey + "_"
)

var (
	extensionName   = filepath.Base(os.Args[0]) // extension name has to match the filename
	extensionClient = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	printPrefix     = fmt.Sprintf("[%s]", extensionName)

	// For the purposes of mocking in tests
	getenv  = os.Getenv
	environ = os.Environ
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		cancel()
		println(printPrefix, "Received", s)
		println(printPrefix, "Exiting")
	}()

	_, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}

	println(printPrefix, "---")
	println(printPrefix, "init")
	println(printPrefix, "---")

	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultAuthRole := os.Getenv("VAULT_AUTH_ROLE")
	vaultAuthProvider := os.Getenv("VAULT_AUTH_PROVIDER")

	configuredSecrets, err := parseConfiguredSecrets()
	if err != nil {
		log.Fatalf("%s: Failed to parse configured secrets to read: %s", printPrefix, err)
	}

	if vaultAddr == "" || vaultAuthProvider == "" || vaultAuthRole == "" || len(configuredSecrets) == 0 {
		println(printPrefix, "missing VAULT_ADDR, VAULT_AUTH_PROVIDER, VAULT_AUTH_ROLE, or VAULT_SECRET_ environment variables.")
		return
	}

	client, err := vaultClient(vaultAuthRole, vaultAuthProvider)
	if err != nil {
		log.Fatalf("%s: error getting client: %s", printPrefix, err)
	} else if client == nil {
		log.Fatalf("%s: nil client returned: %s", printPrefix, err)
	}

	if _, err = os.Stat(defaultSecretDirectory); os.IsNotExist(err) {
		err = os.MkdirAll(defaultSecretDirectory, 0755)
		if err != nil {
			log.Fatalf("Failed to create directory /tmp/vault: %s", err)
		}
	}

	err = ioutil.WriteFile(path.Join(defaultSecretDirectory, "token"), []byte(client.Token()), 0644)
	if err != nil {
		log.Fatal(err)
	}

	for _, s := range configuredSecrets {
		// Will block until shutdown event is received or cancelled via the context.
		secret, err := client.Logical().Read(s.vaultPath)
		if err != nil {
			log.Printf("[%s] error reading secret: %#v", extensionName, err)
			continue
		}

		content, err := json.MarshalIndent(secret, "", "  ")
		if err != nil {
			log.Fatalf("%s: %s", printPrefix, err)
		}
		dir := path.Dir(s.filePath)
		if _, err = os.Stat(dir); os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				log.Fatalf("%s: Failed to create directory %q for secret %s: %s", printPrefix, dir, s.name, err)
			}
		}
		err = ioutil.WriteFile(s.filePath, content, 0644)
		if err != nil {
			log.Fatal(err)
		}

		println(printPrefix, string(content))
	}

	println(printPrefix, "---")
	println(printPrefix, "end init")
	println(printPrefix, "---")
	processEvents(ctx, client)
}

// processEvents polls the Lambda Extension API for events. Currently all this
// does is signal readiness to the Lambda platform after each event, which is
// required in the Extension API.
func processEvents(ctx context.Context, client *api.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			println(printPrefix, "Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				println(printPrefix, "Error:", err)
				println(printPrefix, "Exiting")
				return
			}
			println(printPrefix, "Received event")
			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				println(printPrefix, "Received SHUTDOWN event")
				println(printPrefix, "Exiting")
				return
			}

			if err != nil {
				log.Fatalf("ERROR: %s", err)
			}
			println(printPrefix, "Done with event, next...")
		}
	}
}

// vaultClient uses the AWS IAM auth method configured in a Vault cluster to
// authenticate the execution role and create a Vault API client.
func vaultClient(vaultAuthRole, vaultAuthProvider string) (*api.Client, error) {
	vaultClient, err := api.NewClient(nil)
	if err != nil {
		log.Printf("%s: error making extension: %#v", printPrefix, err)
	}

	ses := session.Must(session.NewSession())
	stsSvc := sts.New(ses)

	// ignore out
	req, _ := stsSvc.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{})
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
	d["role"] = vaultAuthRole

	println(printPrefix, "attemping Vault login...")
	resp, err := vaultClient.Logical().Write(fmt.Sprintf("auth/%s/login", vaultAuthProvider), d)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("got no response from the %s authentication provider", vaultAuthProvider)
	}

	token, err := parseToken(resp)
	if err != nil {
		return nil, fmt.Errorf("error parsing token: %s", err)
	}
	vaultClient.SetToken(token)

	return vaultClient, nil
}

func parseToken(resp *api.Secret) (string, error) {
	var err error
	token, err := resp.TokenID()
	if err != nil {
		return "", err
	}

	return token, nil
}

// ConfiguredSecret represents a pair of environment variables of the form:
//
// VAULT_SECRET_PATH_FOO=/kv/data/foo
// VAULT_SECRET_FILE_FOO=/tmp/vault/secret/foo
//
// Where FOO is the name, and must match across both env vars to form a
// valid secret configuration. The name can also be empty.
type ConfiguredSecret struct {
	name      string // The name assigned to the secret
	vaultPath string // The path to read from in Vault
	filePath  string // The path to write to in the file system
}

// Valid checks that both a secret path and a destination path are given.
func (cs ConfiguredSecret) Valid() bool {
	return cs.vaultPath != "" && cs.filePath != ""
}

// Name is the name parsed from the environment variable name. This name is used
// as a key to match secrets with file paths.
func (cs ConfiguredSecret) Name() string {
	if cs.name == "" {
		return "<anonymous>"
	}

	return cs.name
}

func parseConfiguredSecrets() ([]ConfiguredSecret, error) {
	envVars := environ()
	secrets := make(map[string]*ConfiguredSecret)
	var resultErr error

	for _, kv := range envVars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			// This should never happen.
			return nil, fmt.Errorf("os.Environ should return key=value pairs, but got %s", kv)
		}
		key := parts[0]
		value := parts[1]

		switch {
		case strings.HasPrefix(key, vaultSecretPathPrefix):
			name := key[len(vaultSecretPathPrefix):]
			if name == "" {
				resultErr = multierror.Append(resultErr, fmt.Errorf("%s is not valid configuration; specify %s for a nameless secret or specify a non-zero length name", vaultSecretPathPrefix, vaultSecretPathKey))
				break
			}
			if s, exists := secrets[name]; exists {
				s.vaultPath = value
			} else {
				secrets[name] = &ConfiguredSecret{
					name:      name,
					vaultPath: value,
				}
			}

		case strings.HasPrefix(key, vaultSecretFilePrefix):
			name := key[len(vaultSecretFilePrefix):]
			if name == "" {
				resultErr = multierror.Append(resultErr, fmt.Errorf("%s is not valid configuration; specify %s for a nameless secret or specify a non-zero length name", vaultSecretFilePrefix, vaultSecretFileKey))
				break
			}
			filePath := filePathFromEnv(value)
			if s, exists := secrets[name]; exists {
				s.filePath = filePath
			} else {
				secrets[name] = &ConfiguredSecret{
					name:     name,
					filePath: filePath,
				}
			}
		}
	}

	// Special case for anonymous-name secret
	anonymousSecretVaultPath := getenv(vaultSecretPathKey)
	if anonymousSecretVaultPath != "" {
		s := &ConfiguredSecret{
			name:      "",
			vaultPath: anonymousSecretVaultPath,
			filePath:  filePathFromEnv(getenv(vaultSecretFileKey)),
		}
		if s.filePath == "" {
			s.filePath = path.Join(defaultSecretDirectory, defaultSecretFile)
		}
		secrets[""] = s
	}

	// Track files we will write to check for clashes.
	fileLocations := make(map[string]*ConfiguredSecret)
	result := make([]ConfiguredSecret, 0)
	for _, secret := range secrets {
		if !secret.Valid() {
			resultErr = multierror.Append(resultErr, fmt.Errorf("invalid secret (must have both a path and a file specified): path=%q, file=%q", secret.vaultPath, secret.filePath))
			continue
		}
		if processedSecret, clash := fileLocations[secret.filePath]; clash {
			resultErr = multierror.Append(resultErr, fmt.Errorf("two secrets, %q and %q, are configured to write to the same location on disk: %s", processedSecret.Name(), secret.Name(), secret.filePath))
			continue
		}
		fileLocations[secret.filePath] = secret

		result = append(result, *secret)
	}

	if len(result) == 0 {
		resultErr = multierror.Append(resultErr, fmt.Errorf("no valid secrets to read configured"))
	}

	return result, resultErr
}

func filePathFromEnv(envFilePath string) string {
	if envFilePath == "" {
		return ""
	}
	if path.IsAbs(envFilePath) {
		return envFilePath
	}

	return path.Join(defaultSecretDirectory, envFilePath)
}
