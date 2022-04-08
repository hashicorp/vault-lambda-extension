# Based on vanilla Dockerfile to build a Golang image from:
# https://docs.aws.amazon.com/lambda/latest/dg/go-image.html
# The main modification is to copy the extension binary into the image.
FROM golang:1.17.8 as build

# Cache dependencies
ADD go.mod go.sum /go/src/vault-lambda-extension/
WORKDIR /go/src/vault-lambda-extension/
RUN go mod download

# Build
ADD . .
RUN go build -o /main

# Copy artifacts to a clean image
FROM public.ecr.aws/lambda/provided:al2
COPY --from=build /main /main

# Copy in the extension to a special location where the AWS Lambda Runtime API
# knows to invoke it.
COPY pkg/extensions/vault-lambda-extension /opt/extensions/vault-lambda-extension

ENTRYPOINT [ "/main" ]
