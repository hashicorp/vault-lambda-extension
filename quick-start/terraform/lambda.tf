resource "aws_lambda_function" "function" {
  function_name = "${var.environment_name}-function"
  description   = "Demo Vault AWS Lambda extension"
  role          = aws_iam_role.lambda.arn
  filename      = "../demo-function/demo-function.zip"
  handler       = "main"
  runtime       = "provided.al2"
  architectures = ["x86_64"]
  layers        = var.local_extension ? ["${aws_lambda_layer_version.vle[0].arn}"] : ["arn:aws:lambda:${var.aws_region}:634166935893:layer:vault-lambda-extension:14"]

  environment {
    variables = {
      VAULT_ADDR           = "http://${aws_instance.vault-server.public_ip}:8200",
      VAULT_AUTH_ROLE      = aws_iam_role.lambda.name,
      VAULT_AUTH_PROVIDER  = "aws",
      VAULT_SECRET_PATH_DB = "database/creds/lambda-function",
      VAULT_SECRET_FILE_DB = "/tmp/vault_secret.json",
      VAULT_SECRET_PATH    = "secret/myapp/config",
      VAULT_ASSUMED_ROLE_ARN = var.assume_role ? aws_iam_role.extra_role[0].arn : "",
      DATABASE_URL         = aws_db_instance.main.address
    }
  }
}

// if you have built a local version you want to use
resource "aws_lambda_layer_version" "vle" {
  count = var.local_extension ? 1 : 0
  filename = "../../pkg/vault-lambda-extension.zip"
  layer_name = "vault-lambda-extension"
  compatible_architectures = ["x86_64"]
}