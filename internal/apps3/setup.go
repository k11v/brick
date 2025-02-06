package apps3

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// BucketName is the name of the S3 bucket used by the application.
//
// It is a hard-coded string because the application uses MinIO
// instead of Amazon S3, and MinIO isn't expected to be shared.
//
// It is a also variable instead of a constant because aws-sdk-go-v2 often
// requires a pointer to a string which is easier to acqurie with a variable.
var BucketName = "brick"

// Setup shouldn't be used with AWS as is because it doesn't specify the region.
// See NewClient for connection string format and panic conditions.
func Setup(ctx context.Context, connectionString string) error {
	client := NewClient(connectionString)

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
