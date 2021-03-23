# vault-lambda-extension

This repository contains the source code for HashiCorp's Vault AWS Lambda extension.
The extension utilizes the AWS Lambda Extensions API to help your Lambda function
read secrets from your Vault deployment. To use it, include the following ARN as a
layer in your Lambda function:

```text
arn:aws:lambda:us-east-1:634166935893:layer:vault-lambda-extension:8
```

Where region may be any of `ap-northeast-1`, `ap-northeast-2`, `ap-south-1`,
`ap-southeast-1`, `ap-southeast-2`, `ca-central-1`, `eu-central-1`, `eu-north-1`,
`eu-west-1`, `eu-west-2`, `eu-west-3`, `sa-east-1`, `us-east-1`, `us-east-2`,
`us-west-1`, `us-west-2`.

The extension authenticates with Vault using [AWS IAM auth][vault-aws-iam-auth],
and all configuration is supplied via environment variables. There are two methods
to read secrets, which can both be used side-by-side:

* Configure environment variables such as `VAULT_SECRET_PATH` for the extension
  to read a secret and write it to disk.
* Make unauthenticated requests to the extension's local server at
  `http://127.0.0.1:8200`, which will add an authentication header and proxy to
  the configured `VAULT_ADDR`.

## Getting Started

### Quick Start

The [quick-start](./quick-start) directory has an end to end example, for which
you will need an AWS account and some command line tools. Follow the readme in
that directory if you'd like to try out the extension from scratch. **Please
note it will create real infrastructure with an associated cost as per AWS'
pricing.**

### Adding the extension to your existing Lambda and Vault infrastructure

Requirements:

* ARN of the role your Lambda runs as
* An instance of Vault accessible from AWS Lambda
* An authenticated `vault` client
* A secret in Vault that you want your Lambda to access, and a policy giving read access to it
* **Your Lambda function must use one of the [supported runtimes][lambda-supported-runtimes] for extensions**

First, set up AWS IAM auth on Vault, and attach a policy to your ARN:

```bash
vault auth enable aws
vault write -force auth/aws/config/client
vault write auth/aws/role/vault-lambda-role \
    auth_type=iam \
    bound_iam_principal_arn="${YOUR_ARN}" \
    policies="${YOUR_POLICY}" \
    ttl=1h
```

Add the extension to your Lambda layers using the console or [cli][lambda-add-layer-cli]:

```text
arn:aws:lambda:<your-region>:634166935893:layer:vault-lambda-extension:8
```

Configure the extension using [Lambda environment variables][lambda-env-vars]:

```bash
VAULT_ADDR=http://vault.example.com:8200    # Your Vault address
VAULT_AUTH_PROVIDER=aws                     # The AWS IAM auth mount point, i.e. the path segment after auth/ from above
VAULT_AUTH_ROLE=vault-lambda-role           # The Vault role to authenticate as. Must be configured for the ARN of your Lambda's role
VAULT_SECRET_PATH=secret/lambda-app/token   # The path to a secret in Vault. Can be static or dynamic.
                                            # Unless VAULT_SECRET_FILE is specified, JSON response will be written to /tmp/vault/secret.json
```

If everything is correctly set up, your Lambda function can then read secret
material from `/tmp/vault/secret.json`. The exact contents of the JSON object
will depend on the secret read, but its schema is the [Secret struct][vault-api-secret-struct]
from the Vault API module.

Alternatively, you can send normal Vault API requests over HTTP to the local
proxy at `http://127.0.0.1:8200`, and the extension will add authentication
before forwarding the request. Vault responses will be returned unmodified.

## Configuration

The extension is configured via [Lambda environment variables][lambda-env-vars].
Most of the [Vault CLI client's environment variables][vault-env-vars] are available,
as well as some additional variables to configure auth, which secret(s) to read and
where to write secrets.

Environment variable              | Description | Required | Example value
----------------------------------|-------------|----------|--------------
`VAULT_ADDR`                      | Vault address to connect to | Yes | `https://x.x.x.x:8200`
`VAULT_AUTH_PROVIDER`             | Name of the configured AWS IAM auth route on Vault | Yes | `aws`
`VAULT_AUTH_ROLE`                 | Vault role to authenticate as | Yes | `lambda-app`
`VAULT_IAM_SERVER_ID`             | Value to pass to the Vault server via the [`X-Vault-AWS-IAM-Server-ID` HTTP Header for AWS Authentication](https://www.vaultproject.io/api-docs/auth/aws#iam_server_id_header_value) | No | `vault.example.com`
`VAULT_SECRET_PATH`               | Secret path to read, written to `/tmp/vault/secret.json` unless `VAULT_SECRET_FILE` is specified | No | `database/creds/lambda-app`
`VAULT_SECRET_FILE`               | Path to write the JSON response for `VAULT_SECRET_PATH` | No | `/tmp/db.json`
`VAULT_SECRET_PATH_FOO`           | Additional secret path to read, where FOO can be any name, as long as a matching `VAULT_SECRET_FILE_FOO` is specified | No | `secret/lambda-app/token`
`VAULT_SECRET_FILE_FOO`           | Must exist for any correspondingly named `VAULT_SECRET_PATH_FOO`. Name has no further effect beyond matching to the correct path variable | No | `/tmp/token`
`VAULT_TOKEN_EXPIRY_GRACE_PERIOD` | Period at the end of the proxy server's auth token TTL where it will consider the token expired and attempt to re-authenticate to Vault. Must have a unit and be parseable by `time.Duration`. Defaults to 10s. | No | `1m`

The remaining environment variables are not required, and function exactly as
described in the [Vault Commands (CLI)][vault-env-vars] documentation. However,
note that `VAULT_CLIENT_TIMEOUT` cannot extend the timeout beyond the 10s
initialization timeout imposed by the Extensions API when writing files to disk.

Environment variable    | Description | Required | Example value
------------------------|-------------|----------|--------------
`VAULT_CACERT`          | Path to a PEM-encoded CA certificate _file_ on the local disk | No | `/tmp/ca.crt`
`VAULT_CAPATH`          | Path to a _directory_ of PEM-encoded CA certificate files on the local disk | No | `/tmp/certs`
`VAULT_CLIENT_CERT`     | Path to a PEM-encoded client certificate on the local disk | No | `/tmp/client.crt`
`VAULT_CLIENT_KEY`      | Path to an unencrypted, PEM-encoded private key on disk which corresponds to the matching client certificate | No | `/tmp/client.key`
`VAULT_CLIENT_TIMEOUT`  | Timeout for Vault requests. Default value is 60s. Ignored by proxy server. **Any value over 10s will exceed the Extensions API timeout and therefore have no effect** | No | `5s`
`VAULT_MAX_RETRIES`     | Maximum number of retries on `5xx` error codes. Defaults to 2. Ignored by proxy server | No | `2`
`VAULT_SKIP_VERIFY`     | Do not verify Vault's presented certificate before communicating with it. Setting this variable is not recommended and voids Vault's [security model][vault-security-model]  | No | `true`
`VAULT_TLS_SERVER_NAME` | Name to use as the SNI host when connecting via TLS | No | `vault.example.com`
`VAULT_RATE_LIMIT`      | Only applies to a single invocation of the extension. See [Vault Commands (CLI)][vault-env-vars] documentation for details. Ignored by proxy server | No | `10`
`VAULT_NAMESPACE`       | The namespace to use for pre-configured secrets. Ignored by proxy server | No | `education`

## Limitations

For this early release, the extension does not support automatic secret renewal.
This means once a secret is written to disk, it will not be refreshed once it
expires. This may cause problems if you use [provisioned concurrency][lambda-provisioned-concurrency]
or if your Lambda is invoked often enough that execution contexts live beyond
the lifetime of the secret.

[vault-aws-iam-auth]: https://www.vaultproject.io/docs/auth/aws
[vault-env-vars]: https://www.vaultproject.io/docs/commands#environment-variables
[vault-api-secret-struct]: https://github.com/hashicorp/vault/blob/api/v1.0.4/api/secret.go#L15
[vault-security-model]: https://www.vaultproject.io/docs/internals/security
[lambda-supported-runtimes]: https://docs.aws.amazon.com/lambda/latest/dg/runtimes-extensions-api.html
[lambda-env-vars]: https://docs.aws.amazon.com/lambda/latest/dg/configuration-envvars.html
[lambda-add-layer-cli]: https://docs.aws.amazon.com/lambda/latest/dg/configuration-layers.html#configuration-layers-using
[lambda-provisioned-concurrency]: https://docs.aws.amazon.com/lambda/latest/dg/configuration-concurrency.html#configuration-concurrency-provisioned
