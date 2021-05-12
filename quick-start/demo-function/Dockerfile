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
COPY pkg/extensions/vault-lambda-extension /opt/extensions/vault-lambda-extension
ENTRYPOINT [ "/main" ]