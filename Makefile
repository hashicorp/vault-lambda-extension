GOOS?=linux
GOARCH?=amd64
GOLANG_IMAGE?=docker.mirror.hashicorp.services/golang:1.16.3
CI_TEST_ARGS=
ifdef CI
override CI_TEST_ARGS:=--junitfile=$(TEST_RESULTS_DIR)/go-test/results.xml --jsonfile=$(TEST_RESULTS_DIR)/go-test/results.json
endif

.PHONY: build lint test clean mod quick-start quick-start-destroy

build: clean
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build \
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
	mkdir -p pkg/extensions

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
