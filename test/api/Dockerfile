FROM docker.mirror.hashicorp.services/golang:1.15-alpine3.13 as builder

COPY . /go/src/
WORKDIR /go/src/

RUN go build -o /bin/api main.go

FROM docker.mirror.hashicorp.services/alpine:3.13

COPY --from=builder /bin/api /bin/api

ENTRYPOINT /bin/api