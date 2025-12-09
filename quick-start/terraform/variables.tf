# Copyright IBM Corp. 2020, 2025
# SPDX-License-Identifier: MPL-2.0

# AWS region and AZs in which to deploy
variable "aws_region" {
  default = "us-east-1"
}

# All resources will be tagged with this
variable "environment_name" {
  default = "vault-lambda-extension-demo"
}

# URL for Vault OSS binary
variable "vault_zip_file" {
  default = "https://releases.hashicorp.com/vault/1.19.5/vault_1.19.5_linux_amd64.zip"
}

# Instance size
variable "instance_type" {
  default = "t2.micro"
}

# DB instance size
variable "db_instance_type" {
  default = "db.t3.micro"
}

# true if you want to set and use VAULT_ASSUME_ROLE_ARN
variable "assume_role" {
  type = bool
  default = false
}

# true if you want to use the locally built extension in pkg/vault-lambda-extension.zip
variable "local_extension" {
  type = bool
  default = false
}
