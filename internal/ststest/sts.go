// Copyright IBM Corp. 2020, 2025
// SPDX-License-Identifier: MPL-2.0

package ststest

import (
	"net/http"
	"net/http/httptest"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// FakeSTS creates a fake STS server, and configures the AWS config passed in
// to talk to that server.
func FakeSTS(cfg *aws.Config) (*httptest.Server, aws.Config) {
	cpycfg := *cfg
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	cpycfg.BaseEndpoint = aws.String(stsServer.URL)
	cpycfg.Region = "us-east-1"
	cpycfg.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("foo", "foo", "foo"))

	return stsServer, cpycfg
}
