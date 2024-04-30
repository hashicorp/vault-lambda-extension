## Unreleased

## 0.10.3 (April 30, 2024)

IMPROVEMENTS:
* Bump dependencies (https://github.com/hashicorp/vault-lambda-extension/pull/135):
  * golang.org/x/crypto to v0.22.0
  * golang.org/x/net to v0.24.0
  * golang.org/x/sys to v0.19.0
* Bump dependencies (https://github.com/hashicorp/vault-lambda-extension/pull/134):
  * go to 1.22.2
  * vault api to v1.12.2

## 0.10.2 (February 6, 2024)

LAYERS:
```
arn:aws:lambda:<AWS_REGION>:634166935893:layer:vault-lambda-extension:18
arn:aws:lambda:<AWS_REGION>:634166935893:layer:vault-lambda-extension-arm64:6
```

IMPROVEMENTS:
* Bumped versions for the following dependencies with security vulnerabilities:
  * golang.org/x/crypto v0.18.0
  * golang.org/x/net v0.20.0
  * golang.org/x/sys v0.16.0
  * golang.org/x/text v0.14.0
* Bumped dependencies:
  * github.com/aws/aws-sdk-go v1.50.12
  * github.com/stretchr/testify v1.8.4
  * github.com/hashicorp/vault/api v1.11.0
  * github.com/hashicorp/vault/sdk v0.10.2
  
## 0.10.1 (July 10, 2023)

LAYERS:
```
arn:aws:lambda:<AWS_REGION>:634166935893:layer:vault-lambda-extension:17
arn:aws:lambda:<AWS_REGION>:634166935893:layer:vault-lambda-extension-arm64:5
```

IMPROVEMENTS:
* quick-start: Update Postgres version to 14.7
* Add debug logs during initialization step

## 0.10.0 (March 30, 2023)

LAYERS:
```
arn:aws:lambda:<AWS_REGION>:634166935893:layer:vault-lambda-extension:16
arn:aws:lambda:<AWS_REGION>:634166935893:layer:vault-lambda-extension-arm64:4
```

IMPROVEMENTS:
* Requests from the extension to Vault now set the User-Agent field accordingly.
* Introduced a `VAULT_RUN_MODE` environment variable to allow user to run in proxy mode, file mode, or both. 
  * The default value is 'default', which runs in *both* proxy and file mode.
* Vault Lambda Extension version dynamically injected at build time.

## 0.9.0 (February 23, 2023)

IMPROVEMENTS:
* Building with Go 1.19.6
* Bumped versions for the following dependencies with security vulnerabilities:
  * github.com/hashicorp/vault/api v1.9.0
  * github.com/hashicorp/vault/sdk v0.8.1
  * golang.org/x/net v0.7.0
  * golang.org/x/sys v0.5.0
  * golang.org/x/text v0.7.0

## 0.8.0 (August 22, 2022)

FEATURES:

* `VAULT_ASSUMED_ROLE_ARN` can be used to specify a role for your lambda function to assume. [[GH-69](https://github.com/hashicorp/vault-lambda-extension/pull/69)]

IMPROVEMENTS:

* Bumped versions for the following dependencies with security vulnerabilities:
  * golang.org/x/crypto to v0.0.0-20220817201139-bc19a97f63c8
  * golang.org/x/net to v0.0.0-20220812174116-3211cb980234
  * golang.org/x/sys to v0.0.0-20220818161305-2296e01440c6
  * golang.org/x/text to v0.3.7

## 0.7.0 (April 26, 2022)

CHANGES:

* Static function code can now reliably read secrets written to disk, because extension registration now occurs after writing files. [[GH-61](https://github.com/hashicorp/vault-lambda-extension/pull/61)]
* arm64 architecture now supported [[GH-67](https://github.com/hashicorp/vault-lambda-extension/pull/67)]

## 0.6.0 (March 14, 2022)

CHANGES:

* Logging is now levelled and less chatty by default. Level can be controlled by VAULT_LOG_LEVEL environment variable. [[GH-63](https://github.com/hashicorp/vault-lambda-extension/pull/63)]

FEATURES:

* Add caching support in the local proxy server [[GH-58](https://github.com/hashicorp/vault-lambda-extension/pull/58))

IMPROVEMENTS:

* Leading and trailing whitespace is trimmed from environment variable values on reading. [[GH-63](https://github.com/hashicorp/vault-lambda-extension/pull/63)]

## 0.5.0 (August 24, 2021)

FEATURES:

* Use client-controlled consistency when writing secrets to disk to reliably support performance standbys/replicas. [[GH-47](https://github.com/hashicorp/vault-lambda-extension/pull/47)]

## 0.4.0 (July 1, 2021)

FEATURES:

* Add `VAULT_STS_ENDPOINT_REGION` environment variable to make STS regional endpoint used for auth configurable separately from the region Lambda is deployed in. [[GH-30](https://github.com/hashicorp/vault-lambda-extension/pull/30)]
* Add `VLE_VAULT_ADDR` environment variable to configure Vault address to connect to. Allows clients of the proxy to consume the standard `VAULT_ADDR`. [[GH-41](https://github.com/hashicorp/vault-lambda-extension/pull/41)]

DOCUMENTATION:

* Added documentation for deploying the extension into `Image` format Lambdas [[GH-34](https://github.com/hashicorp/vault-lambda-extension/pull/34)]
* Added documentation on performance impact of extension [[GH-35](https://github.com/hashicorp/vault-lambda-extension/pull/35)]
* Added documentation on uploading the extension into different accounts and regions [[GH-37](https://github.com/hashicorp/vault-lambda-extension/pull/37)]

## v0.3.0 (March 23rd, 2021)

FEATURES:

* Proxy server mode: The extension now starts a Vault API proxy at
  `http://127.0.0.1:8200` [[GH-27](https://github.com/hashicorp/vault-lambda-extension/pull/27)]
  * **Breaking change:** The extension no longer writes its own Vault auth token
    to disk. Writing pre-configured secrets to disk remains unchanged.

## v0.2.0 (January 20th, 2021)

IMPROVEMENTS:

* Add Vault IAM Server ID login header if set. [[GH-21](https://github.com/hashicorp/vault-lambda-extension/pull/21)]
* quick-start: Make db_instance_type configurable.

## v0.1.0 (October 8th, 2020)

Features:

* Initial release
