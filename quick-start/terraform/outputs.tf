output "info" {
  value = <<EOF

Vault Server IP (public): ${aws_instance.vault-server.public_ip}
Vault UI URL:             http://${aws_instance.vault-server.public_ip}:8200/ui

You can SSH into the Vault EC2 instance using private.key:
    ssh -i private.key ubuntu@${aws_instance.vault-server.public_ip}

You are now ready to run your Lambda function. Assuming `aws` is configured to
use the same account that terraform deployed to (configured by the AWS_...
environment variables), you can run:

aws lambda invoke --function-name ${aws_lambda_function.function.function_name} /dev/null \
    --cli-binary-format raw-in-base64-out \
    --log-type Tail \
    --region ${var.aws_region} \
    | jq -r '.LogResult' \
    | base64 --decode

EOF
}
