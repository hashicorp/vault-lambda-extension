# Copyright IBM Corp. 2020, 2025
# SPDX-License-Identifier: MPL-2.0

FROM docker.io/hashicorp/vault:1.21.1 AS vault

FROM docker.mirror.hashicorp.services/golang:1.26-alpine AS builder

COPY . /go/src/
WORKDIR /go/src/

RUN go build -o /bin/vault-lambda-extension main.go

FROM docker.mirror.hashicorp.services/alpine:3.13

RUN apk update && apk add curl

COPY --from=vault /bin/vault /bin/vault
COPY --from=builder /bin/vault-lambda-extension /opt/extensions/vault-lambda-extension
COPY test/lambda/runtime.sh /bin/runtime.sh

ENTRYPOINT ["/bin/runtime.sh"]