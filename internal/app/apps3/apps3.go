package apps3

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	transport "github.com/aws/smithy-go/endpoints"
)

// BucketName is the name of the S3 bucket used by the application.
//
// It is a hard-coded string because the application uses MinIO
// instead of Amazon S3, and MinIO isn't expected to be shared.
//
// It is a also variable instead of a constant because aws-sdk-go-v2 often
// requires a pointer to a string which is easier to acqurie with a variable.
var BucketName = "brick"

// endpointResolver implements s3.EndpointResolverV2.
// It resolves endpoints for S3-compatible object storage like MinIO.
type endpointResolver struct {
	BaseURL *url.URL // required
}

func (r *endpointResolver) ResolveEndpoint(_ context.Context, params s3.EndpointParameters) (transport.Endpoint, error) {
	u := *r.BaseURL
	u.Path += "/" + *params.Bucket
	return transport.Endpoint{URI: u}, nil
}

// NewClient creates a new Client using the provided connection string.
// The connection string must be a valid URL in the format: http://key:secret@s3:9000.
// For MinIO, the key and secret are the username and password respectively.
// It panics if the connection string is not a valid URL.
func NewClient(connectionString string) *s3.Client {
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
			EndpointResolverV2: &endpointResolver{BaseURL: u},
		},
	)
	return client
}

// Setup shouldn't be used with AWS as is because it doesn't specify the region.
func Setup(ctx context.Context, client *s3.Client) error {
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &BucketName,
	})
	if ownedErr := (*types.BucketAlreadyOwnedByYou)(nil); errors.As(err, &ownedErr) {
		// continue
	} else if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	err = s3.NewBucketExistsWaiter(client).Wait(
		ctx,
		&s3.HeadBucketInput{Bucket: &BucketName},
		time.Minute,
	)
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	return nil
}
