package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/hashicorp/vault-lambda-extension/config"
	"github.com/hashicorp/vault-lambda-extension/extension"
	"github.com/hashicorp/vault-lambda-extension/vault"
)

const (
	extensionName = "vault-lambda-extension"
)

var (
	extensionClient = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
)

func main() {
	logger := log.New(os.Stderr, fmt.Sprintf("[%s] ", extensionName), log.Ldate|log.Ltime|log.LUTC)
	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		cancel()
		logger.Println("Received", s)
		logger.Println("Exiting")
	}()

	_, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		logger.Fatal(err)
	}

	initialiseExtension(logger)

	processEvents(ctx, logger)
}

func initialiseExtension(logger *log.Logger) {
	logger.Println("Initialising")

	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultAuthRole := os.Getenv("VAULT_AUTH_ROLE")
	vaultAuthProvider := os.Getenv("VAULT_AUTH_PROVIDER")
	vaultIamServerId := os.Getenv("VAULT_IAM_SERVER_ID")	// Optional

	configuredSecrets, err := config.ParseConfiguredSecrets()
	if err != nil {
		logger.Fatalf("Failed to parse configured secrets to read: %s", err)
	}

	if vaultAddr == "" || vaultAuthProvider == "" || vaultAuthRole == "" || len(configuredSecrets) == 0 {
		logger.Fatal("missing VAULT_ADDR, VAULT_AUTH_PROVIDER, VAULT_AUTH_ROLE, or VAULT_SECRET_ environment variables.")
	}

	client, err := vault.NewClient(logger, vaultAuthRole, vaultAuthProvider, vaultIamServerId)
	if err != nil {
		logger.Fatalf("error getting client: %s", err)
	} else if client == nil {
		logger.Fatalf("nil client returned: %s", err)
	}

	if _, err = os.Stat(config.DefaultSecretDirectory); os.IsNotExist(err) {
		err = os.MkdirAll(config.DefaultSecretDirectory, 0755)
		if err != nil {
			logger.Fatalf("Failed to create directory %s: %s", config.DefaultSecretDirectory, err)
		}
	}

	err = ioutil.WriteFile(path.Join(config.DefaultSecretDirectory, "token"), []byte(client.Token()), 0644)
	if err != nil {
		logger.Fatal(err)
	}

	for _, s := range configuredSecrets {
		// Will block until shutdown event is received or cancelled via the context.
		secret, err := client.Logical().Read(s.VaultPath)
		if err != nil {
			logger.Fatalf("error reading secret: %s", err)
		}

		content, err := json.MarshalIndent(secret, "", "  ")
		if err != nil {
			logger.Fatalf("%s", err)
		}
		dir := path.Dir(s.FilePath)
		if _, err = os.Stat(dir); os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				logger.Fatalf("Failed to create directory %q for secret %s: %s", dir, s.Name(), err)
			}
		}
		err = ioutil.WriteFile(s.FilePath, content, 0644)
		if err != nil {
			logger.Fatal(err)
		}
	}

	logger.Println("Initialised")
}

// processEvents polls the Lambda Extension API for events. Currently all this
// does is signal readiness to the Lambda platform after each event, which is
// required in the Extension API.
func processEvents(ctx context.Context, logger *log.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Println("Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				logger.Fatalf("Error receiving event: %s", err)
			}
			logger.Println("Received event")
			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				logger.Println("Received SHUTDOWN event, exiting")
				return
			}
		}
	}
}
