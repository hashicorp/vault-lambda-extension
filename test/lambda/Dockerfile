FROM docker.mirror.hashicorp.services/vault:1.6.2 as vault

FROM docker.mirror.hashicorp.services/golang:1.15-alpine3.13 as builder

COPY . /go/src/
WORKDIR /go/src/

RUN go build -o /bin/vault-lambda-extension main.go

FROM docker.mirror.hashicorp.services/alpine:3.13

RUN apk update && apk add curl

COPY --from=vault /bin/vault /bin/vault
COPY --from=builder /bin/vault-lambda-extension /opt/extensions/vault-lambda-extension
COPY test/lambda/runtime.sh /bin/runtime.sh

ENTRYPOINT ["/bin/runtime.sh"]