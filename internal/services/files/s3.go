package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type S3 struct {
	client *s3.Client
	bucket string
}

func NewS3(endpoint, access, secret, bucket, region string) (Objects, error) {
	if bucket == "" {
		return nil, nil
	}
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || bucket == "" || access == "" || secret == "" {
		return nil, fmt.Errorf("S3 requires a valid endpoint, bucket and credentials")
	}
	if region == "" {
		region = "us-east-1"
	}
	client := s3.New(s3.Options{BaseEndpoint: aws.String(endpoint), Region: region, Credentials: credentials.NewStaticCredentialsProvider(access, secret, ""), UsePathStyle: true, HTTPClient: &http.Client{Timeout: 60 * time.Second}})
	return &S3{client, bucket}, nil
}
func missing(e error) bool {
	var a smithy.APIError
	return errors.As(e, &a) && (a.ErrorCode() == "NotFound" || a.ErrorCode() == "NoSuchKey" || a.ErrorCode() == "404")
}
func (s *S3) Ensure(ctx context.Context, key string, body []byte) error {
	_, e := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if e == nil {
		return nil
	}
	if !missing(e) {
		return fmt.Errorf("S3 head: %w", e)
	}
	_, e = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(body)})
	if e != nil {
		return fmt.Errorf("S3 put: %w", e)
	}
	return nil
}

// Delete removes the directory marker and all immutable revisions of this file.
func (s *S3) Delete(ctx context.Context, key string) error {
	parts := strings.Split(key, "/")
	if len(parts) < 3 || parts[0] != "files" {
		return ErrInvalid
	}
	prefix := "files/" + parts[1] + "/"
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: &prefix})
	for pages.HasMorePages() {
		page, e := pages.NextPage(ctx)
		if e != nil {
			return e
		}
		for _, obj := range page.Contents {
			_, e = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: obj.Key})
			if e != nil && !missing(e) {
				return e
			}
		}
	}
	return nil
}

// Exists refuses a metadata-only update when the bytes must be supplied again.
func (s *S3) Exists(ctx context.Context, key string) error {
	_, e := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if missing(e) {
		return ErrConflict
	}
	return e
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if missing(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("S3 get: %w", err)
	}
	return out.Body, nil
}
