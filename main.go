package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault-lambda-extension/internal/extension"
	"github.com/hashicorp/vault-lambda-extension/internal/proxy"
	"github.com/hashicorp/vault-lambda-extension/internal/vault"
	"github.com/hashicorp/vault/api"
)

const (
	extensionName = "vault-lambda-extension"
)

func main() {
	logger := log.New(os.Stderr, fmt.Sprintf("[%s] ", extensionName), log.Ldate|log.Ltime|log.LUTC)
	ctx, cancel := context.WithCancel(context.Background())
	extensionClient := extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	_, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		logger.Fatal(err)
	}

	var wg sync.WaitGroup
	srv, err := runExtension(ctx, logger, &wg)
	if err != nil {
		logger.Fatal(err)
	}

	shutdownChannel := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, syscall.SIGTERM, syscall.SIGINT)
		select {
		case s := <-interruptChannel:
			logger.Printf("Received %s, exiting\n", s)
		case <-shutdownChannel:
			logger.Println("Received shutdown event, exiting")
		}

		cancel()
		if err := srv.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			logger.Printf("HTTP server shutdown error: %s\n", err)
		}
	}()

	processEvents(ctx, logger, extensionClient)

	// Once processEvents returns, signal that it's time to shutdown.
	shutdownChannel <- struct{}{}

	// Ensure we wait for the HTTP server to gracefully shut down.
	wg.Wait()
	logger.Println("Graceful shutdown complete")
}

func runExtension(ctx context.Context, logger *log.Logger, wg *sync.WaitGroup) (*http.Server, error) {
	logger.Println("Initialising")

	authConfig := config.AuthConfigFromEnv()
	vaultConfig := api.DefaultConfig()
	if vaultConfig.Error != nil {
		return nil, fmt.Errorf("error making default vault config for extension: %w", vaultConfig.Error)
	}

	if authConfig.VaultAddress != "" {
		vaultConfig.Address = authConfig.VaultAddress
	}

	if vaultConfig.Address == "" || authConfig.Provider == "" || authConfig.Role == "" {
		return nil, errors.New("missing VLE_VAULT_ADDR, VAULT_ADDR, VAULT_AUTH_PROVIDER or VAULT_AUTH_ROLE environment variables")
	}

	var ses *session.Session
	if authConfig.STSEndpointRegion != "" {
		ses = session.Must(session.NewSession(&aws.Config{
			Region:              aws.String(authConfig.STSEndpointRegion),
			STSRegionalEndpoint: endpoints.RegionalSTSEndpoint,
		}))
	} else {
		ses = session.Must(session.NewSession())
	}
	client, err := vault.NewClient(logger, vaultConfig, authConfig, ses)
	if err != nil {
		return nil, fmt.Errorf("error getting client: %w", err)
	} else if client == nil {
		return nil, fmt.Errorf("nil client returned: %w", err)
	}

	var newState string
	// Leverage Vault helpers for eventual consistency on login
	client.VaultClient = client.VaultClient.WithResponseCallbacks(api.RecordState(&newState))
	_, err = client.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("error logging in to Vault: %w", err)
	}

	client.VaultClient = client.VaultClient.WithRequestCallbacks(api.RequireState(newState)).WithResponseCallbacks()

	err = writePreconfiguredSecrets(client.VaultClient)
	if err != nil {
		return nil, err
	}

	// clear out eventual consistency helpers
	client.VaultClient = client.VaultClient.WithRequestCallbacks().WithResponseCallbacks()

	ln, err := net.Listen("tcp", "127.0.0.1:8200")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port 8200: %w", err)
	}
	srv := proxy.New(logger, client, config.CacheConfigFromEnv())
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Println("Starting HTTP server")
		err = srv.Serve(ln)
		if err != http.ErrServerClosed {
			logger.Printf("HTTP server shutdown unexpectedly: %s\n", err)
		}
	}()

	logger.Println("Initialised")

	return srv, nil
}

func writePreconfiguredSecrets(client *api.Client) error {
	configuredSecrets, err := config.ParseConfiguredSecrets()
	if err != nil {
		return fmt.Errorf("failed to parse configured secrets to read: %w", err)
	}

	for _, s := range configuredSecrets {
		// Will block until shutdown event is received or cancelled via the context.
		secret, err := client.Logical().Read(s.VaultPath)
		if err != nil {
			return fmt.Errorf("error reading secret: %w", err)
		}

		content, err := json.MarshalIndent(secret, "", "  ")
		if err != nil {
			return err
		}
		dir := path.Dir(s.FilePath)
		if _, err = os.Stat(dir); os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				return fmt.Errorf("failed to create directory %q for secret %s: %s", dir, s.Name(), err)
			}
		}
		err = ioutil.WriteFile(s.FilePath, content, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

// processEvents polls the Lambda Extension API for events. Currently all this
// does is signal readiness to the Lambda platform after each event, which is
// required in the Extension API.
// The first call to NextEvent signals completion of the extension
// init phase.
func processEvents(ctx context.Context, logger *log.Logger, extensionClient *extension.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Println("Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				logger.Printf("Error receiving event: %s\n", err)
				return
			}
			logger.Println("Received event")
			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				return
			}
		}
	}
}
