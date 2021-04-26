package config

import "os"

const (
	vaultAuthRole     = "VAULT_AUTH_ROLE"
	vaultAuthProvider = "VAULT_AUTH_PROVIDER"
	vaultIAMServerID  = "VAULT_IAM_SERVER_ID"
)

// AuthConfig holds config required for logging in to Vault.
type AuthConfig struct {
	Role        string
	Provider    string
	IAMServerID string
}

// AuthConfigFromEnv reads config from the environment for authenticating to Vault.
func AuthConfigFromEnv() AuthConfig {
	return AuthConfig{
		Role:        os.Getenv(vaultAuthRole),
		Provider:    os.Getenv(vaultAuthProvider),
		IAMServerID: os.Getenv(vaultIAMServerID), // Optional
	}
}
