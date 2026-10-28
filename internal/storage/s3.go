package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Options says how to reach an S3-compatible bucket such as OCI Object Storage.
// Credentials are deliberately not part of it: they come from the standard AWS environment
// variables (AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY), so they never live in a config file.
type S3Options struct {
	// Endpoint is the S3-compatible endpoint. Empty means the real AWS S3.
	Endpoint string
	// Region is needed to sign requests. For OCI use the region identifier, for example af-casablanca-1.
	Region string
	Bucket string
}

// S3Store is a Store backed by an S3-compatible bucket.
type S3Store struct {
	client *s3.Client
	bucket string
}

var _ Store = (*S3Store)(nil)

// NewS3Store builds a client for the bucket. It does not contact the network.
func NewS3Store(ctx context.Context, opts S3Options) (*S3Store, error) {
	var errs []error
	if opts.Bucket == "" {
		errs = append(errs, errors.New("bucket is required"))
	}
	if opts.Region == "" {
		errs = append(errs, errors.New("region is required (for OCI, the region identifier such as af-casablanca-1)"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("s3 store: %w", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(opts.Region))
	if err != nil {
		return nil, fmt.Errorf("s3 store: load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
			// OCI's compatibility endpoint uses path-style addressing: endpoint/bucket/key.
			o.UsePathStyle = true
		}
		// Since service/s3 v1.73.0 the SDK adds checksum headers by default, which breaks some
		// non-AWS providers (SignatureDoesNotMatch on GetObject). We only read, so ask for
		// checksums only where the API requires them.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	return &S3Store{client: client, bucket: opts.Bucket}, nil
}

// Latest returns the newest object under prefix, or ErrNoBackup.
// It walks every page of the listing, because S3 returns at most 1000 keys per page.
// Keys ending in "/" are folder placeholders created by consoles, not backups, so they are skipped.
func (s *S3Store) Latest(ctx context.Context, prefix string) (Object, error) {
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	var best Object
	found := false
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return Object{}, fmt.Errorf("list objects under %q: %w", prefix, err)
		}
		for _, o := range page.Contents {
			if o.Key == nil || o.LastModified == nil || strings.HasSuffix(*o.Key, "/") {
				continue
			}
			if !found || o.LastModified.After(best.LastModified) {
				best = Object{Key: *o.Key, Size: aws.ToInt64(o.Size), LastModified: *o.LastModified}
				found = true
			}
		}
	}
	if !found {
		return Object{}, ErrNoBackup
	}
	return best, nil
}

// Download streams the object with the given key into dst.
func (s *S3Store) Download(ctx context.Context, key string, dst io.Writer) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get object %q: %w", key, err)
	}
	defer out.Body.Close()

	n, err := io.Copy(dst, out.Body)
	if err != nil {
		return fmt.Errorf("download %q: %w", key, err)
	}
	if out.ContentLength != nil && n != *out.ContentLength {
		return fmt.Errorf("download %q: received %d bytes, expected %d", key, n, *out.ContentLength)
	}
	return nil
}
