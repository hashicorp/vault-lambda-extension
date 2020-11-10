# AWS region and AZs in which to deploy
variable "aws_region" {
  default = "us-east-1"
}

# AWS zone
variable "aws_zone" {
  default = "us-east-1a"
}

variable "aws_zone_alternative" {
  default = "us-east-1b"
}

# All resources will be tagged with this
variable "environment_name" {
  default = "vault-lambda-extension-demo"
}

# URL for Vault OSS binary
variable "vault_zip_file" {
  default = "https://releases.hashicorp.com/vault/1.5.4/vault_1.5.4_linux_amd64.zip"
}

# Instance size
variable "instance_type" {
  default = "t2.micro"
}

# DB instance size
variable "db_instance_type" {
  default = "db.t2.micro"
}

variable "vpc_cidr" {
  type        = string
  description = "CIDR of the VPC"
  default     = "172.30.0.0/16"
}

variable "subnet_cidr_one" {
  type        = string
  description = "CIDR of the VPC"
  default     = "172.30.1.0/24"
}

variable "subnet_cidr_two" {
  type        = string
  description = "CIDR of the VPC"
  default     = "172.30.2.0/24"
}
