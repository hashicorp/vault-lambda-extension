# `test/api`

In AWS Lambda functions, both extensions and the function itself operate by
requesting the next event from an HTTP API in an infinite loop. Each event
returned is typically either an invocation, or for extensions, it could also
be a request to shutdown.

This folder contains a small API server to mock that AWS Lambda API for the
purpose of local/CI integration tests. It's intentionally as small and as
simplistic as possible to support the bare minimum to run extension
binaries.

In addition to the AWS Lambda APIs, there is also a /_sync endpoint used
within the tests to synchronise on events such as the extension signalling
readiness.
