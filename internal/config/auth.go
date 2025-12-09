// Copyright IBM Corp. 2020, 2025
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"os"
	"strings"
)

const (
	vaultAuthRole        = "VAULT_AUTH_ROLE"
	vaultAuthProvider    = "VAULT_AUTH_PROVIDER"
	vaultAssumedRoleArn  = "VAULT_ASSUMED_ROLE_ARN"    // Optional
	vaultIAMServerID     = "VAULT_IAM_SERVER_ID"       // Optional
	vleVaultAddr         = "VLE_VAULT_ADDR"            // Optional, overrides VAULT_ADDR
	stsEndpointRegionEnv = "VAULT_STS_ENDPOINT_REGION" // Optional
)

// AuthConfig holds config required for logging in to Vault.
type AuthConfig struct {
	Role              string
	Provider          string
	AssumedRoleArn    string
	IAMServerID       string
	STSEndpointRegion string
	VaultAddress      string
}

// AuthConfigFromEnv reads config from the environment for authenticating to Vault.
func AuthConfigFromEnv() AuthConfig {
	return AuthConfig{
		Role:              strings.TrimSpace(os.Getenv(vaultAuthRole)),
		Provider:          strings.TrimSpace(os.Getenv(vaultAuthProvider)),
		AssumedRoleArn:    strings.TrimSpace(os.Getenv(vaultAssumedRoleArn)),
		IAMServerID:       strings.TrimSpace(os.Getenv(vaultIAMServerID)),
		STSEndpointRegion: strings.TrimSpace(os.Getenv(stsEndpointRegionEnv)),
		VaultAddress:      strings.TrimSpace(os.Getenv(vleVaultAddr)),
	}
}
