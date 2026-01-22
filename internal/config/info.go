// Copyright IBM Corp. 2020, 2025
// SPDX-License-Identifier: MPL-2.0

package config

const (
	ExtensionName = "vault-lambda-extension"
	VaultLogLevel = "VAULT_LOG_LEVEL" // Optional, one of TRACE, DEBUG, INFO, WARN, ERROR, OFF
	VaultRunMode  = "VAULT_RUN_MODE"
)

var (
	// ExtensionVersion should be a var type, so the go build tool can override and inject a custom version.
	ExtensionVersion = "0.0.0-dev"
)
