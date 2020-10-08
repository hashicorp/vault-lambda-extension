#!/bin/sh

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
echo "Tagging and publishing Vault Lambda layer to ${region}..."

GIT_TAG=$1
if [ -z $GIT_TAG ]
then
      echo "empty git tag"
      exit 1
fi

if output=$(git status --porcelain --untracked-files=no) && [ -z "$output" ]; then
  echo "Tagging ${GIT_TAG}"
else 
  echo "Working directory is not clean, exiting"
  exit 1
fi

REGIONS=(
  ap-northeast-1
  ap-northeast-2
  ap-south-1
  ap-southeast-1
  ap-southeast-2
  ca-central-1
  eu-central-1
  eu-west-1
  eu-west-2
  eu-west-3
  sa-east-1
  us-east-1
  us-east-2
  us-west-1
  us-west-2
)

echo ""
echo "Building layer..."
mkdir -p pkg/extensions
# in case this already exists, clear it out prior to zip
rm -f pkg/extensions/*.* 2> /dev/null

# Create temporary magic-file, to activate extensions in multiple regions. This
# can be removed after general availability of the API
touch pkg/preview-extensions-ggqizro707

GOOS=linux GOARCH=amd64 go build -ldflags '-s -w' -a -o pkg/extensions/vault-lambda-extension main.go
echo ""

pushd pkg
rm -f extensions.zip 2> /dev/null
zip -r extensions.zip extensions preview-extensions-ggqizro707

LAYER_NAME="vault-lambda-extension"

# Tag after we've successfully built
echo "Git Tag: ${GIT_TAG}"

MSG="Vault AWS Lambda Extension ${GIT_TAG}"

git tag -a "${GIT_TAG}" -m "${MSG}"
git push --follow-tags origin ${GIT_TAG}

for region in "${REGIONS[@]}"; do
    echo "Publishing Vault Lambda layer to ${region}..."
    layer_version=$(aws lambda publish-layer-version \
        --layer-name $LAYER_NAME \
        --zip-file  "fileb://extensions.zip" \
        --description "${MSG}" \
        --region $region \
        --output text \
        --query Version)
    echo "published Vault Lambda extension layer version ${layer_version} to ${region}"

    echo "Setting public permissions for Vault Lambda extension layer version ${layer_version} in ${region}"
    output=$(aws lambda add-layer-version-permission \
      --layer-name $LAYER_NAME \
      --version-number $layer_version \
      --statement-id public \
      --action lambda:GetLayerVersion \
      --principal "*" \
      --region $region)
    echo "Public permissions set for Vault Lambda extension layer version ${layer_version} in region ${region}"
done
popd
echo "...done"
