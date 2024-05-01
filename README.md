# vault-lambda-extension

----

**Please note**: We take Vault's security and our users' trust very seriously. If you believe you have found a security issue in Vault or vault-lambda-extension, _please responsibly disclose_ by contacting us at [security@hashicorp.com](mailto:security@hashicorp.com).

----

This repository contains the source code for HashiCorp's Vault AWS Lambda extension.
The extension utilizes the AWS Lambda Extensions API to help your Lambda function
read secrets from your Vault deployment.

## Usage

To use the extension, include one of the following ARNs as a layer in your
Lambda function, depending on your desired architecture.

amd64 (x86_64):

```text
arn:aws:lambda:<your-region>:634166935893:layer:vault-lambda-extension:19
```

arm64:

```text
arn:aws:lambda:<your-region>:634166935893:layer:vault-lambda-extension-arm64:7
```

Where region may be any of
  * `af-south-1`
  * `ap-east-1`
  * `ap-northeast-1`
  * `ap-northeast-2`
  * `ap-northeast-3`
  * `ap-south-1`
  * `ap-south-2`
  * `ap-southeast-1`
  * `ap-southeast-2`
  * `ca-central-1`
  * `eu-central-1`
  * `eu-north-1`
  * `eu-south-1`
  * `eu-west-1`
  * `eu-west-2`
  * `eu-west-3`
  * `me-south-1`
  * `sa-east-1`
  * `us-east-1`
  * `us-east-2`
  * `us-west-1`
  * `us-west-2`

Alternatively, you can download binaries for packaging into a container image
[here][releases]. See the full [documentation page][vault-docs] for more details.

The extension authenticates with Vault using [AWS IAM auth][vault-aws-iam-auth],
and all configuration is supplied via environment variables. There are two methods
to read secrets, which can both be used side-by-side:

* **Recommended**: Make unauthenticated requests to the extension's local proxy
  server at `http://127.0.0.1:8200`, which will add an authentication header and
  proxy to the configured `VAULT_ADDR`. Responses from Vault are returned without
  modification.
* Configure environment variables such as `VAULT_SECRET_PATH` for the extension
  to read a secret and write it to disk.

## Getting Started

The [learn guide][vault-learn-guide] is the most complete and fully explained
tutorial on getting started from scratch. Alternatively, you can follow the
similar quick start guide below or see the instructions for adding the extension
to your existing function. General [usage documentation][vault-docs] is also
available.

### Quick Start

The [quick-start](./quick-start) directory has an end to end example, for which
you will need an AWS account and some command line tools. Follow the readme in
that directory if you'd like to try out the extension from scratch. **Please
note it will create real infrastructure with an associated cost as per AWS'
pricing.**

## Testing

If you want to test changes to the lambda extension, you can build and deploy the local version for testing with the Quick Start:

```sh
make zip
make quick-start TERRAFORM_ARGS="-var local_extension=true"
```

There is also a terraform variable for using an additional IAM role for the Lambda to assume:

```
make zip
make quick-start TERRAFORM_ARGS="-var local_extension=true -var assume_role=true"
```

[vault-learn-guide]: https://learn.hashicorp.com/tutorials/vault/aws-lambda
[vault-docs]: https://developer.hashicorp.com/vault/docs/platform/aws/lambda-extension
[vault-aws-iam-auth]: https://developer.hashicorp.com/vault/docs/auth/aws
[releases]: https://releases.hashicorp.com/vault-lambda-extension/
