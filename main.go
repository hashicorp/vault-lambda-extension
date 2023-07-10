// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"

	"github.com/hashicorp/vault-lambda-extension/internal/config"
	"github.com/hashicorp/vault-lambda-extension/internal/extension"
	"github.com/hashicorp/vault-lambda-extension/internal/proxy"
	"github.com/hashicorp/vault-lambda-extension/internal/runmode"
	"github.com/hashicorp/vault-lambda-extension/internal/vault"
)

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Level: hclog.LevelFromString(os.Getenv(config.VaultLogLevel)),
	})

	logger.Info(fmt.Sprintf("Starting Vault Lambda Extension %v", config.ExtensionVersion))
	runMode := runmode.ModeDefault
	if runModeEnv := os.Getenv(config.VaultRunMode); runModeEnv != "" {
		runMode = runmode.ParseMode(runModeEnv)
	}

	h := newHandler(logger.Named(config.ExtensionName), runMode)
	if err := h.handle(); err != nil {
		logger.Error("Fatal error, exiting", "error", err)
		os.Exit(1)
	}
}

func newHandler(logger hclog.Logger, runMode runmode.Mode) *handler {
	return &handler{
		logger:  logger,
		runMode: runMode,
	}
}

type handler struct {
	logger  hclog.Logger
	runMode runmode.Mode
}

func (h *handler) handle() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	cleanup, err := h.runExtension(ctx, &wg)
	if err != nil {
		return err
	}

	shutdownChannel := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, syscall.SIGTERM, syscall.SIGINT)
		select {
		case sig := <-interruptChannel:
			h.logger.Info("Received signal, exiting", "signal", sig)
		case <-shutdownChannel:
			h.logger.Info("Received shutdown event, exiting")
		}

		cancel()
		if err := cleanup(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			h.logger.Error("HTTP server shutdown error", "error", err)
		}
	}()

	extensionClient := extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	_, err = extensionClient.Register(ctx, config.ExtensionName)
	if err != nil {
		return err
	}

	processEvents(ctx, h.logger, extensionClient)

	// Once processEvents returns, signal that it's time to shutdown.
	shutdownChannel <- struct{}{}

	// Ensure we wait for the HTTP server to gracefully shut down.
	wg.Wait()
	h.logger.Info("Graceful shutdown complete")

	return nil
}

func (h *handler) runExtension(ctx context.Context, wg *sync.WaitGroup) (func(context.Context) error, error) {
	start := time.Now()
	h.logger.Info("Initialising")

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
	client, err := vault.NewClient(config.ExtensionName, config.ExtensionVersion, h.logger.Named("vault-client"), vaultConfig, authConfig, ses)
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

	uaFunc := func(request *api.Request) string {
		return config.GetUserAgentBase(config.ExtensionName, config.ExtensionVersion) + "; writing to temp file"
	}

	client.VaultClient = client.VaultClient.WithRequestCallbacks(api.RequireState(newState), vault.UserAgentRequestCallback(uaFunc)).WithResponseCallbacks()

	if h.runMode.HasModeFile() {
		if err := writePreconfiguredSecrets(h.logger, client.VaultClient); err != nil {
			return nil, err
		}
	}

	// clear out eventual consistency helpers
	client.VaultClient = client.VaultClient.WithRequestCallbacks().WithResponseCallbacks()

	cleanupFunc := func(context.Context) error { return nil }
	if h.runMode.HasModeProxy() {
		start := time.Now()
		h.logger.Debug("initialising proxy mode")
		ln, err := net.Listen("tcp", "127.0.0.1:8200")
		if err != nil {
			return nil, fmt.Errorf("failed to listen on port 8200: %w", err)
		}
		srv := proxy.New(h.logger.Named("proxy"), client, config.CacheConfigFromEnv())
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.logger.Info("Starting HTTP proxy server")
			err = srv.Serve(ln)
			if err != http.ErrServerClosed {
				h.logger.Error("HTTP server shutdown unexpectedly", "error", err)
			}
		}()
		cleanupFunc = func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		}
		h.logger.Debug(fmt.Sprintf("proxy mode initialised in %v", time.Since(start)))
	}

	h.logger.Info(fmt.Sprintf("Initialised in %v", time.Since(start)))
	return cleanupFunc, nil
}

// writePreconfiguredSecrets writes secrets to disk.
func writePreconfiguredSecrets(logger hclog.Logger, client *api.Client) error {
	start := time.Now()
	logger.Debug("writing secrets to disk")
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
			return fmt.Errorf("unable to marshal json: %w", err)
		}

		dir := path.Dir(s.FilePath)
		if _, err = os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %q for secret %s: %s", dir, s.Name(), err)
			}
		}

		if err := os.WriteFile(s.FilePath, content, 0644); err != nil {
			return fmt.Errorf("error writing file: %w", err)
		}
	}

	logger.Debug(fmt.Sprintf("wrote secrets to disk in %v", time.Since(start)))
	return nil
}

// processEvents polls the Lambda Extension API for events. Currently all this
// does is signal readiness to the Lambda platform after each event, which is
// required in the Extension API.
// The first call to NextEvent signals completion of the extension
// init phase.
func processEvents(ctx context.Context, logger hclog.Logger, extensionClient *extension.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info("Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				logger.Error("Error receiving event", "error", err)
				return
			}
			logger.Info("Received event")
			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				return
			}
		}
	}
}
