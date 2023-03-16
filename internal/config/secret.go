// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/hashicorp/go-multierror"
)

const (
	vaultSecretPathKey    = "VAULT_SECRET_PATH"
	vaultSecretFileKey    = "VAULT_SECRET_FILE"
	vaultSecretPathPrefix = vaultSecretPathKey + "_"
	vaultSecretFilePrefix = vaultSecretFileKey + "_"

	DefaultSecretDirectory = "/tmp/vault"
	DefaultSecretFile      = "secret.json"
)

var (
	// For the purposes of mocking in tests
	getenv  = os.Getenv
	environ = os.Environ
)

// ConfiguredSecret represents a pair of environment variables of the form:
//
// VAULT_SECRET_PATH_FOO=/kv/data/foo
// VAULT_SECRET_FILE_FOO=/tmp/vault/secret/foo
//
// Where FOO is the name, and must match across both env vars to form a
// valid secret configuration. The name can also be empty.
type ConfiguredSecret struct {
	name string // The name assigned to the secret

	VaultPath string // The path to read from in Vault
	FilePath  string // The path to write to in the file system
}

// Valid checks that both a secret path and a destination path are given.
func (cs ConfiguredSecret) Valid() bool {
	return cs.VaultPath != "" && cs.FilePath != ""
}

// Name is the name parsed from the environment variable name. This name is used
// as a key to match secrets with file paths.
func (cs ConfiguredSecret) Name() string {
	if cs.name == "" {
		return "<anonymous>"
	}

	return cs.name
}

// ParseConfiguredSecrets reads environment variables to determine which secrets
// to read from Vault, and where to write them on disk.
func ParseConfiguredSecrets() ([]ConfiguredSecret, error) {
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
		value := strings.TrimSpace(parts[1])

		switch {
		case strings.HasPrefix(key, vaultSecretPathPrefix):
			name := key[len(vaultSecretPathPrefix):]
			if name == "" {
				resultErr = multierror.Append(resultErr, fmt.Errorf("%s is not valid configuration; specify %s for a nameless secret or specify a non-zero length name", vaultSecretPathPrefix, vaultSecretPathKey))
				break
			}
			if s, exists := secrets[name]; exists {
				s.VaultPath = value
			} else {
				secrets[name] = &ConfiguredSecret{
					name:      name,
					VaultPath: value,
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
				s.FilePath = filePath
			} else {
				secrets[name] = &ConfiguredSecret{
					name:     name,
					FilePath: filePath,
				}
			}
		}
	}

	// Special case for anonymous-name secret
	anonymousSecretVaultPath := strings.TrimSpace(getenv(vaultSecretPathKey))
	if anonymousSecretVaultPath != "" {
		s := &ConfiguredSecret{
			name:      "",
			VaultPath: anonymousSecretVaultPath,
			FilePath:  filePathFromEnv(strings.TrimSpace(getenv(vaultSecretFileKey))),
		}
		if s.FilePath == "" {
			s.FilePath = path.Join(DefaultSecretDirectory, DefaultSecretFile)
		}
		secrets[""] = s
	}

	// Track files we will write to check for clashes.
	fileLocations := make(map[string]*ConfiguredSecret)
	result := make([]ConfiguredSecret, 0)
	for _, secret := range secrets {
		if !secret.Valid() {
			resultErr = multierror.Append(resultErr, fmt.Errorf("invalid secret (must have both a path and a file specified): path=%q, file=%q", secret.VaultPath, secret.FilePath))
			continue
		}
		if processedSecret, clash := fileLocations[secret.FilePath]; clash {
			resultErr = multierror.Append(resultErr, fmt.Errorf("two secrets, %q and %q, are configured to write to the same location on disk: %s", processedSecret.Name(), secret.Name(), secret.FilePath))
			continue
		}
		fileLocations[secret.FilePath] = secret

		result = append(result, *secret)
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

	return path.Join(DefaultSecretDirectory, envFilePath)
}
