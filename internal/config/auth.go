package config

import (
	"os"
	"strings"
)

const (
	vaultAuthRole        = "VAULT_AUTH_ROLE"
	vaultAuthProvider    = "VAULT_AUTH_PROVIDER"
	vaultIAMServerID     = "VAULT_IAM_SERVER_ID"       // Optional
	vleVaultAddr         = "VLE_VAULT_ADDR"            // Optional, overrides VAULT_ADDR
	stsEndpointRegionEnv = "VAULT_STS_ENDPOINT_REGION" // Optional
)

// AuthConfig holds config required for logging in to Vault.
type AuthConfig struct {
	Role              string
	Provider          string
	IAMServerID       string
	STSEndpointRegion string
	VaultAddress      string
}

// AuthConfigFromEnv reads config from the environment for authenticating to Vault.
func AuthConfigFromEnv() AuthConfig {
	return AuthConfig{
		Role:              strings.TrimSpace(os.Getenv(vaultAuthRole)),
		Provider:          strings.TrimSpace(os.Getenv(vaultAuthProvider)),
		IAMServerID:       strings.TrimSpace(os.Getenv(vaultIAMServerID)),
		STSEndpointRegion: strings.TrimSpace(os.Getenv(stsEndpointRegionEnv)),
		VaultAddress:      strings.TrimSpace(os.Getenv(vleVaultAddr)),
	}
}
