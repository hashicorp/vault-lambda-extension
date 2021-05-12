# demo-function

This directory contains a demo function for the purposes of testing the
vault-lambda-extension. It attempts to connect and query users from a database
using two different sets of credentials; one set fetched from disk (which the
extension wrote during initialization) and the other set from the proxy server
which is queried during the function runtime.

The `quick-start` guide defaults to deploying this function in a zip format
Lambda function. See the readme in the parent `quick-start` folder for full
instructions on deploying it.

Alternatively, to deploy the `quick-start` guide using a container format Lambda,
make the following modifications:

* Create an ECR repository. You can use AWS CLI or console, or add this to
  the Terraform config:

  ```hcl
  resource "aws_ecr_repository" "demo-function" {
    name                 = "demo-function"
  }
  ```

* Package the extension and function together into a Docker image. From this
  directory:

  ```bash
  pushd ../../ && GOOS=linux GOARCH=amd64 go build -ldflags '-s -w' -a -o quick-start/demo-function/pkg/extensions/vault-lambda-extension main.go && popd
  docker build -t demo-function .
  ```

* Tag the image, and push it to your new ECR repository:

  ```bash
  export AWS_ACCOUNT="ACCOUNT HERE"
  export AWS_REGION="REGION HERE"
  docker tag demo-function:latest ${AWS_ACCOUNT?}.dkr.ecr.${AWS_REGION?}.amazonaws.com/demo-function:latest
  docker push ${AWS_ACCOUNT?}.dkr.ecr.${AWS_REGION?}.amazonaws.com/demo-function:latest
  ```

* Update `quick-start/terraform/lambda.tf` to use your image
  * Set `image_uri = "<account>.dkr.ecr.<region>.amazonaws.com/demo-function:latest"`
  * Set `package_type  = "Image"`
  * Unset `filename`, `handler`, `runtime`, and `layers`
