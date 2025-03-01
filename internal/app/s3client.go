package app

import (
	"context"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	transport "github.com/aws/smithy-go/endpoints"
)

// NewS3Client creates a new Client using the provided connection string.
// The connection string must be a valid URL in the format: http://key:secret@s3:9000.
// For MinIO, the key and secret are the username and password respectively.
// It panics if the connection string is not a valid URL.
func NewS3Client(connectionString string) *s3.Client {
	u, err := url.Parse(connectionString)
	if err != nil {
		panic(err)
	}

	username := u.User.Username()
	password, _ := u.User.Password()
	u.User = nil

	client := s3.New(
		s3.Options{
			Credentials:        credentials.NewStaticCredentialsProvider(username, password, ""),
			EndpointResolverV2: &s3EndpointResolver{BaseURL: u},
		},
	)
	return client
}

// s3EndpointResolver implements s3.EndpointResolverV2.
// It resolves endpoints for S3-compatible object storage like MinIO.
type s3EndpointResolver struct {
	BaseURL *url.URL // required
}

func (r *s3EndpointResolver) ResolveEndpoint(_ context.Context, params s3.EndpointParameters) (transport.Endpoint, error) {
	u := *r.BaseURL
	u.Path += "/" + *params.Bucket
	return transport.Endpoint{URI: u}, nil
}
