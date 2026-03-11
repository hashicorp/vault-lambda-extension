# Copyright IBM Corp. 2020, 2025
# SPDX-License-Identifier: MPL-2.0

//--------------------------------------------------------------------
// Resources

## Vault Server IAM Config
resource "aws_iam_instance_profile" "vault-server" {
  name = "${var.environment_name}-vault-server-instance-profile"
  role = aws_iam_role.vault-server.name
}

resource "aws_iam_role" "vault-server" {
  name               = "${var.environment_name}-vault-server-role"
  assume_role_policy = data.aws_iam_policy_document.assume_role_ec2.json
}

resource "aws_iam_role_policy" "vault-server" {
  name   = "${var.environment_name}-vault-server-role-policy"
  role   = aws_iam_role.vault-server.id
  policy = data.aws_iam_policy_document.vault-server.json
}

# Vault Client (Lambda function) IAM Config
resource "aws_iam_role" "lambda" {
  name               = "${var.environment_name}-lambda-role"
  assume_role_policy = var.assume_role ? data.aws_iam_policy_document.assume_role_lambda_plus_root[0].json : data.aws_iam_policy_document.assume_role_lambda.json
}

resource "aws_iam_role" "extra_role" {
  count              = var.assume_role ? 1 : 0
  name               = "${var.environment_name}-extra-role"
  assume_role_policy = data.aws_iam_policy_document.assume_role_lambda_plus_root[0].json
}

resource "aws_iam_role_policy" "lambda" {
  name   = "${var.environment_name}-lambda-policy"
  role   = aws_iam_role.lambda.id
  policy = var.assume_role ? data.aws_iam_policy_document.lambda_plus_assume_role[0].json : data.aws_iam_policy_document.lambda.json
}

resource "aws_iam_role_policy" "extra_role_policy" {
  count  = var.assume_role ? 1 : 0
  name   = "${var.environment_name}-extra-role-policy"
  role   = aws_iam_role.extra_role[0].id
  policy = data.aws_iam_policy_document.lambda_plus_assume_role[0].json
}

//--------------------------------------------------------------------
// Data Sources

data "aws_iam_policy_document" "assume_role_ec2" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "assume_role_lambda" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "assume_role_lambda_plus_root" {
  count = var.assume_role ? 1 : 0

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    principals {
      type = "AWS"
      identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"]
    }
  }
}

data "aws_iam_policy_document" "vault-server" {
  statement {
    sid    = "ConsulAutoJoin"
    effect = "Allow"

    actions = ["ec2:DescribeInstances"]

    resources = ["*"]
  }

  statement {
    sid    = "VaultAWSAuthMethod"
    effect = "Allow"
    actions = [
      "ec2:DescribeInstances",
      "iam:GetInstanceProfile",
      "iam:GetUser",
      "iam:GetRole",
    ]
    resources = ["*"]
  }

  statement {
    sid    = "VaultKMSUnseal"
    effect = "Allow"

    actions = [
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:DescribeKey",
    ]

    resources = ["*"]
  }
}

data "aws_iam_policy_document" "lambda" {
  statement {
    sid    = "LambdaLogs"
    effect = "Allow"

    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]

    resources = ["*"]
  }
}

data "aws_iam_policy_document" "lambda_plus_assume_role" {
  count = var.assume_role ? 1 : 0

  statement {
    sid    = "LambdaLogs"
    effect = "Allow"

    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]

    resources = ["*"]
  }

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    resources = ["*"]
  }
}
