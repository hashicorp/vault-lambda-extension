# Copyright IBM Corp. 2020, 2025
# SPDX-License-Identifier: MPL-2.0

//--------------------------------------------------------------------
// Providers

provider "aws" {
  // Credentials set via env vars
  region  = var.aws_region
}

//--------------------------------------------------------------------
// Data Sources

data "aws_ami" "ubuntu" {
  most_recent = true

  filter {
    name   = "name"
    values = ["hc-base-ubuntu-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["888995627335"] # Canonical
}

