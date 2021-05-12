# Based on vanilla Dockerfile to build a Golang image from:
# https://docs.aws.amazon.com/lambda/latest/dg/go-image.html
# The only significant modification is to copy the extension binary into the image.
FROM public.ecr.aws/lambda/provided:al2 as build

# Install Golang
RUN yum install -y golang
RUN go env -w GOPROXY=direct

# Cache dependencies
ADD go.mod go.sum ./
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
