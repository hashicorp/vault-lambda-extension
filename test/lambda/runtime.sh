#!/bin/sh
# This script is for local integration testing.
# It simulates a basic version of how Lambda runs a function that includes extensions.

set -euo pipefail

function configure_vault() {
    # Wait until vault is up (retry up to 10 x 1s).
    curl --silent --retry 10 --retry-connrefused --retry-delay 1 $VAULT_ADDR > /dev/null 2>&1

    export VAULT_TOKEN=root
    vault policy write test_policy - <<EOF
    path "secret/*" {
        capabilities = ["read", "list"]
    }
EOF
    vault auth enable aws
    vault write -force auth/aws/config/client
    vault write auth/aws/role/lambda-demo-function \
        auth_type=iam \
        policies=test_policy \
        token_ttl=10s \
        token_max_ttl=15s \
        resolve_aws_unique_ids=false \
        bound_iam_principal_arn="${AWS_ROLE_ARN}"
    vault kv put secret/foo bar=baz
    unset VAULT_TOKEN
}

# Configure Vault
echo "Configuring Vault"
configure_vault

# Run the extension in the background
echo "Running extension"
/opt/extensions/vault-lambda-extension 2>&1 | tee /tmp/vault-lambda-extension.log &
EXTENSION_PID=$!

# Wait for the extension to call /event/next API endpoint, signalling the end of the
# extension initialisation phase.
echo "Waiting for the extension to signal readiness"
curl --silent --max-time 10 api:80/_sync/extension-initialised

# Here is where the function code itself would now be invoked.
# Instead of running a function, we run some tests on the extension.

# Check the extension proxy is working (auth-less call to localhost).
# Also ensure it continues to work beyond the original login token's TTL
# and beyond the original token's max TTL.
echo "Reading secret via proxy server"
curl --silent -H "X-Vault-Request: true" http://127.0.0.1:8200/v1/secret/data/foo
sleep 5

# This loop spans a period where we should see a renewal and the end of the max token TTL.
for i in `seq 15`; do
    curl --silent -H "X-Vault-Request: true" http://127.0.0.1:8200/v1/secret/data/foo > /dev/null
    sleep 1
done

# Tell the API that we're ready for it to send the SHUTDOWN event to the extension.
echo "Signalling shutdown to extension"
curl --silent --request POST api:80/_sync/shutdown-extension

# Wait for the extension to finish shutting down.
wait $EXTENSION_PID
