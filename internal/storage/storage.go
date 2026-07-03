// Package storage abstracts media file storage. Files go to an
// S3-compatible bucket when configured, otherwise to local disk served by
// the backend at /media/. Metadata always lives in PostgreSQL.
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage interface {
	// Put stores the object and returns its public URL.
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) (string, error)
	Delete(ctx context.Context, key string) error
}

// --- S3-compatible ---

type s3Storage struct {
	client *minio.Client
	bucket string
	region string
	host   string
}

func NewS3(endpoint, region, bucket, accessKey, secretKey string) (Storage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: !strings.HasPrefix(endpoint, "localhost") && !strings.HasPrefix(endpoint, "127.0.0.1"),
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}
	return &s3Storage{client: client, bucket: bucket, region: region, host: endpoint}, nil
}

func (s *s3Storage) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("upload to s3: %w", err)
	}
	if s.host == "s3.amazonaws.com" {
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key), nil
	}
	scheme := "https"
	if strings.HasPrefix(s.host, "localhost") || strings.HasPrefix(s.host, "127.0.0.1") {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, s.host, s.bucket, key), nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// --- Local disk ---

type localStorage struct {
	dir     string
	baseURL string // e.g. http://localhost:8080/media
}

func NewLocal(dir, baseURL string) (Storage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &localStorage{dir: dir, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

func (l *localStorage) Put(_ context.Context, key, _ string, r io.Reader, _ int64) (string, error) {
	path := filepath.Join(l.dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return l.baseURL + "/" + key, nil
}

func (l *localStorage) Delete(_ context.Context, key string) error {
	return os.Remove(filepath.Join(l.dir, filepath.FromSlash(key)))
}
