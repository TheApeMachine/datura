package s3

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"
)

/*
Config selects how an S3-backed blob bucket is opened.

Use BucketURL with a URL accepted by blob.OpenBucket (see
https://gocloud.dev/howto/blob/). Example:

	s3://my-bucket?region=us-east-1

For S3-compatible endpoints (MinIO, etc.), use query parameters documented for
aws.V2ConfigFromURLParams — for example endpoint, disableSSL, s3ForcePathStyle.

Alternatively, set Bucket and Region to use the AWS SDK default credential chain
with s3blob.OpenBucket.
*/
type Config struct {
	BucketURL string

	Bucket string
	Region string

	Prefix string
}

/*
Client exposes a portable gocloud *blob.Bucket for S3. Close the client when
shutting down.
*/
type Client struct {
	bucket *blob.Bucket
}

/*
NewClient opens an S3 bucket per https://gocloud.dev/howto/blob/.

When BucketURL is non-empty, it is passed to blob.OpenBucket and must not be
combined with Bucket or Region.

When BucketURL is empty, Bucket and Region are required; the AWS SDK v2 default
configuration is loaded for that region.
*/
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	bucketURL := strings.TrimSpace(cfg.BucketURL)
	bucketName := strings.TrimSpace(cfg.Bucket)
	region := strings.TrimSpace(cfg.Region)
	prefix := normalizePrefix(strings.TrimSpace(cfg.Prefix))

	if bucketURL != "" && (bucketName != "" || region != "") {
		return nil, fmt.Errorf("s3: bucket_url cannot be set together with bucket or region")
	}

	var (
		opened *blob.Bucket
		err    error
	)

	switch {
	case bucketURL != "":
		opened, err = blob.OpenBucket(ctx, bucketURL)

		if err != nil {
			return nil, fmt.Errorf("s3: open bucket url: %w", err)
		}

	default:
		if bucketName == "" {
			return nil, fmt.Errorf("s3: bucket name required when bucket_url is empty")
		}

		if region == "" {
			return nil, fmt.Errorf("s3: region required when bucket_url is empty")
		}

		awsConfiguration, loadErr := config.LoadDefaultConfig(ctx, config.WithRegion(region))

		if loadErr != nil {
			return nil, fmt.Errorf("s3: load aws config: %w", loadErr)
		}

		s3service := s3.NewFromConfig(awsConfiguration)

		opened, err = s3blob.OpenBucket(ctx, s3service, bucketName, nil)

		if err != nil {
			return nil, fmt.Errorf("s3: open bucket: %w", err)
		}
	}

	if prefix != "" {
		opened = blob.PrefixedBucket(opened, prefix)
	}

	return &Client{bucket: opened}, nil
}

/*
Bucket returns the portable blob bucket for reads, writes, and listing.
*/
func (client *Client) Bucket() *blob.Bucket {
	return client.bucket
}

/*
Close releases resources held by the bucket.
*/
func (client *Client) Close() error {
	return client.bucket.Close()
}

func normalizePrefix(prefix string) string {
	if prefix == "" {
		return ""
	}

	if strings.HasSuffix(prefix, "/") {
		return prefix
	}

	return prefix + "/"
}
