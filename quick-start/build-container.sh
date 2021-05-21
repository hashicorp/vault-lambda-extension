#!/bin/sh
# This script builds the demo-function and vault-lambda-extension into a Docker
# container. Use if you would like to deploy the demo as an image instead of
# a zip. See the quick-start readme for more details.

set -euo pipefail

# First, build vault-lambda-extension from source.
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
pushd "${DIR}/../"
GOOS=linux GOARCH=amd64 go build -ldflags '-s -w' -a -o quick-start/demo-function/pkg/extensions/vault-lambda-extension main.go
popd

# Build the container to be uploaded to Lambda, which will build demo-function
# from source and copy vault-lambda-extension into the correct folder.
docker build --file "${DIR}/demo-function/Dockerfile" --tag demo-function "${DIR}/demo-function/"
