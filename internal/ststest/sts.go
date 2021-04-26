package ststest

import (
	"net/http"
	"net/http/httptest"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

// FakeSTS creates a fake STS server, and configures the session passed in
// to talk to that server.
func FakeSTS(ses *session.Session) *httptest.Server {
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	ses.Config.
		WithEndpoint(stsServer.URL).
		WithRegion("us-east-1").
		WithCredentials(credentials.NewStaticCredentialsFromCreds(credentials.Value{
			ProviderName:    session.EnvProviderName,
			AccessKeyID:     "foo",
			SecretAccessKey: "foo",
			SessionToken:    "foo",
		}))

	return stsServer
}
