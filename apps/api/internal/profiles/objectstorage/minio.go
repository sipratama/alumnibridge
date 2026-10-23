package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var objectKey = regexp.MustCompile(`^photos/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type Store struct {
	client *minio.Client
	bucket string
}

func New(ctx context.Context, endpoint, access, secret, bucket string, secure bool) (*Store, error) {
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: secure, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 5 * time.Second, IdleConnTimeout: 30 * time.Second}})
	if err != nil {
		return nil, err
	}
	exists, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err = c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil && minio.ToErrorResponse(err).Code != "BucketAlreadyOwnedByYou" {
			return nil, err
		}
	}
	// Require an empty bucket policy; use IAM identity policies for authenticated access.
	policy, err := c.GetBucketPolicy(ctx, bucket)
	if err != nil {
		code := minio.ToErrorResponse(err).Code
		if code != "NoSuchBucketPolicy" {
			return nil, err
		}
	}
	if policy != "" {
		return nil, errors.New("profile photo bucket must have no anonymous policy")
	}
	return &Store{client: c, bucket: bucket}, nil
}
func (s *Store) Put(ctx context.Context, key string, body []byte, kind string) error {
	if !objectKey.MatchString(key) {
		return errors.New("invalid object key")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: kind, DisableMultipart: true})
	return err
}
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if !objectKey.MatchString(key) {
		return nil, errors.New("invalid object key")
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err = obj.Stat(); err != nil {
		obj.Close()
		return nil, err
	}
	return obj, nil
}
func (s *Store) Delete(ctx context.Context, key string) error {
	if !objectKey.MatchString(key) {
		return errors.New("invalid object key")
	}
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
