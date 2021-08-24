## Unreleased

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
