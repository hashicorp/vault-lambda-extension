# Local integration tests

To run locally:

1. Set up AWS credentials via the following environment variables:

    ```text
    export AWS_ACCESS_KEY_ID=...
    export AWS_SECRET_ACCESS_KEY=...
    export AWS_SESSION_TOKEN=...
    ```

1. Set `AWS_ROLE_ARN` to the ARN of your AWS credentials' role, e.g.:

    ```text
    export AWS_ROLE_ARN=arn:aws:iam::<ACCOUNT_ID>:role/<ROLE_NAME>
    ```

1. Run `docker-compose`. It will exit with error code 0 if successful:

    ```sh
    docker-compose up --build --exit-code lambda
    ```
