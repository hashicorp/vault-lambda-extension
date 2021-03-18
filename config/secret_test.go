package config

import (
	"fmt"
	"sort"
	"testing"

	"github.com/hashicorp/go-multierror"
)

func TestParseConfiguredSecrets(t *testing.T) {
	for _, tc := range []struct {
		name         string
		env          map[string]string
		expected     []ConfiguredSecret
		expectErrors int
	}{
		{
			name:         "Empty environment",
			expected:     []ConfiguredSecret{},
			expectErrors: 0,
		},
		{
			name: "Minimal valid config",
			env: map[string]string{
				"VAULT_SECRET_PATH": "/kv/data/foo",
			},
			expected: []ConfiguredSecret{
				ConfiguredSecret{
					name:      "",
					VaultPath: "/kv/data/foo",
					FilePath:  "/tmp/vault/secret.json",
				},
			},
		},
		{
			name: "1 secret - no name",
			env: map[string]string{
				"VAULT_SECRET_PATH": "/kv/data/foo",
				"VAULT_SECRET_FILE": "/tmp/vault/secret/foo",
			},
			expected: []ConfiguredSecret{
				ConfiguredSecret{
					name:      "",
					VaultPath: "/kv/data/foo",
					FilePath:  "/tmp/vault/secret/foo",
				},
			},
		},
		{
			name: "2 secrets",
			env: map[string]string{
				"VAULT_SECRET_PATH":     "/kv/data/foo",
				"VAULT_SECRET_FILE":     "/tmp/vault/secret/foo",
				"VAULT_SECRET_PATH_FOO": "FOO vaultPath",
				"VAULT_SECRET_FILE_FOO": "/FOO/file/path",
			},
			expected: []ConfiguredSecret{
				ConfiguredSecret{
					name:      "",
					VaultPath: "/kv/data/foo",
					FilePath:  "/tmp/vault/secret/foo",
				},
				ConfiguredSecret{
					name:      "FOO",
					VaultPath: "FOO vaultPath",
					FilePath:  "/FOO/file/path",
				},
			},
		},
		{
			name: "Absolute vs relative paths",
			env: map[string]string{
				"VAULT_SECRET_PATH":          "default location",
				"VAULT_SECRET_PATH_ABSOLUTE": "a",
				"VAULT_SECRET_FILE_ABSOLUTE": "/somewhere/else/completely",
				"VAULT_SECRET_PATH_RELATIVE": "a",
				"VAULT_SECRET_FILE_RELATIVE": "my-special-location.yaml",
			},
			expected: []ConfiguredSecret{
				ConfiguredSecret{
					name:      "",
					VaultPath: "default location",
					FilePath:  "/tmp/vault/secret.json",
				},
				ConfiguredSecret{
					name:      "ABSOLUTE",
					VaultPath: "a",
					FilePath:  "/somewhere/else/completely",
				},
				ConfiguredSecret{
					name:      "RELATIVE",
					VaultPath: "a",
					FilePath:  "/tmp/vault/my-special-location.yaml",
				},
			},
		},
		{
			name: "Misconfigured secrets",
			env: map[string]string{
				"VAULT_SECRET_PATH_FOO":      "a", // No VAULT_SECRET_FILE_FOO env var
				"VAULT_SECRET_PATH_BAR":      "a", // No VAULT_SECRET_PATH_BAR env var
				"VAULT_SECRET_PATH_":         "invalid name",
				"VAULT_SECRET_FILE_":         "invalid name",
				"VAULT_SECRET_PATH":          "a",
				"VAULT_SECRET_PATH_DUP_PATH": "a",
				"VAULT_SECRET_FILE_DUP_PATH": "/tmp/vault/secret.json", // Writes to the same path as the anonymous secret
			},
			expected:     []ConfiguredSecret{},
			expectErrors: 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(tc.env)
			secrets, err := ParseConfiguredSecrets()
			if err != nil {
				if tc.expectErrors == 0 {
					t.Fatalf("Expected no errors, but got: %s", err)
				}
				if merr, ok := err.(*multierror.Error); ok {
					if len(merr.Errors) != tc.expectErrors {
						t.Fatalf("Expected %d error(s) but got %d: %s", tc.expectErrors, len(merr.Errors), err)
					}
				} else if tc.expectErrors != 1 {
					t.Fatalf("Expected %d errors but got 1", tc.expectErrors)
				}
			}
			if err == nil && tc.expectErrors > 0 {
				t.Fatalf("Expected %d errors but got none", tc.expectErrors)
			}

			if tc.expectErrors > 0 {
				return
			}

			if len(secrets) != len(tc.expected) {
				t.Fatalf("Expected %d secret(s), but got %d: %+v", len(tc.expected), len(secrets), secrets)
			}
			sort.Slice(secrets, func(i, j int) bool {
				return secrets[i].name < secrets[j].name
			})
			for i, s := range secrets {
				if s != tc.expected[i] {
					t.Fatalf("Expected secret %+v but got %+v", tc.expected[i], s)
				}
			}
		})
	}
}

func setenv(env map[string]string) {
	getenv = func(k string) string {
		return env[k]
	}
	environ = func() []string {
		result := make([]string, 0, len(env))
		for k, v := range env {
			result = append(result, fmt.Sprintf("%s=%s", k, v))
		}
		return result
	}
}
