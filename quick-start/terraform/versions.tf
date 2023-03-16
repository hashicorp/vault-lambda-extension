# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


terraform {
  required_version = ">= 0.12"
  required_providers {
    aws = {
      version = "~> 4.24.0"
    }
  }
}
