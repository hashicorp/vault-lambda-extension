output "info" {
  value = <<EOF

Infrastructure info:

Vault Server IP (public): ${aws_instance.vault-server.public_ip}
Vault UI URL:             http://${aws_instance.vault-server.public_ip}:8200/ui

If you provided github_auth_org and github_auth_team variables
to terraform, you can log in to Vault using a GitHub PAT with read:org permission.

If you write your own lambda, it should run using this role: ${aws_iam_role.lambda.arn}
It should also set these environment variables, ready for the extension to consume:
    VAULT_ADDR=http://${aws_instance.vault-server.public_ip}:8200
    VAULT_AUTH_ROLE=dev-role-iam
    VAULT_AUTH_PROVIDER=aws
    VAULT_SECRET_PATH=database/creds/lambda-ext-readonly

When using the extension, your lambda should have the database credentials rendered into /tmp/vault_secret.json

Connecting manually to some of the infrastructure:

    SSH into Vault EC2 instance using private.key:
        ssh -i private.key ubuntu@${aws_instance.vault-server.public_ip}

    Manually connect to RDS:
        export VAULT_ADDR=http://${aws_instance.vault-server.public_ip}:8200
        vault login -method=github token=<gituhb-PAT-with-read:org>
        vault read database/creds/lambda-ext-readonly
        psql -d lambdadb -h ${aws_db_instance.main.address} --user <username-from-previous-line>
        <type-password-at-prompt>

You are now ready to run your Lambda function.
Assuming `aws` is logged in to the same account that terraform deployed to
(configured by the AWS_... environment variables), you can run:

aws lambda invoke --function-name ${aws_lambda_function.function.function_name} /dev/null \
    --cli-binary-format raw-in-base64-out \
    --log-type Tail \
    --region ${var.aws_region} \
    | jq -r '.LogResult' \
    | base64 --decode

EOF
}
