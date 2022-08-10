GOOS?=linux
GOARCH?=amd64
CI_TEST_ARGS=

.PHONY: build lint test clean mod quick-start quick-start-destroy publish-layer-version

build: clean
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build \
		-ldflags '-s -w' \
		-a -o pkg/extensions/vault-lambda-extension \
		.

lint:
	golangci-lint run -v --concurrency 2 \
		--disable-all \
		--timeout 10m \
		--enable gofmt \
		--enable gosimple \
		--enable govet

test:
	gotestsum --format=short-verbose $(CI_TEST_ARGS)

clean:
	-rm -rf pkg

mod:
	@go mod tidy

quick-start:
	bash quick-start/build.sh
	cd quick-start/terraform && \
		terraform init && \
		terraform apply -auto-approve
	aws lambda invoke --function-name vault-lambda-extension-demo-function /dev/null \
		--cli-binary-format raw-in-base64-out \
		--log-type Tail \
		--region us-east-1 \
		| jq -r '.LogResult' \
		| base64 --decode

quick-start-destroy:
	cd quick-start/terraform && \
		terraform destroy -auto-approve

publish-layer-version: build
	cd pkg && zip -r vault-lambda-extension.zip extensions/
	aws lambda publish-layer-version \
		--layer-name "vault-lambda-extension" \
		--zip-file "fileb://pkg/vault-lambda-extension.zip" \
		--region "us-east-1" \
		--no-cli-pager \
		--output text \
		--query LayerVersionArn
