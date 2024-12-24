package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storage implements backup storage for S3-compatible services
type S3Storage struct {
	client *s3.Client
	config *S3Config
}

// S3Config holds the configuration for S3-compatible storage
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Path            string
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(config *S3Config) (*S3Storage, error) {
	debugLog("Creating S3 storage with config: %+v", config)

	// Create custom resolver if endpoint is specified (for B2, Minio, etc.)
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID && config.Endpoint != "" {
			return aws.Endpoint{
				URL:               config.Endpoint,
				SigningRegion:     config.Region,
				HostnameImmutable: true,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	// Create AWS config
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(config.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.SecretAccessKey,
			"",
		)),
		awsconfig.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg)

	return &S3Storage{
		client: client,
		config: config,
	}, nil
}

// Upload uploads a file to S3 storage
func (s *S3Storage) Upload(localPath, remoteName string) error {
	debugLog("Uploading %s to %s", localPath, remoteName)

	// Open local file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	// Create S3 key (path + filename)
	key := filepath.Join(s.config.Path, remoteName)
	key = strings.TrimPrefix(key, "./") // Remove ./ prefix if present

	// Upload file
	_, err = s.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	debugLog("Upload completed successfully")
	return nil
}

// Download downloads a file from S3 storage
func (s *S3Storage) Download(remoteName, localPath string) error {
	debugLog("Downloading %s to %s", remoteName, localPath)

	// Create S3 key (path + filename)
	key := filepath.Join(s.config.Path, remoteName)
	key = strings.TrimPrefix(key, "./") // Remove ./ prefix if present

	// Get object from S3
	result, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}
	defer result.Body.Close()

	// Create local file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Copy file contents
	if _, err := io.Copy(file, result.Body); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	return nil
}

// List lists all backup files in S3 storage with the given prefix
func (s *S3Storage) List(prefix string) ([]BackupFile, error) {
	debugLog("Listing files with prefix: %s", prefix)

	// Create S3 prefix (path + prefix)
	s3Prefix := filepath.Join(s.config.Path, prefix)
	s3Prefix = strings.TrimPrefix(s3Prefix, "./") // Remove ./ prefix if present

	var backups []BackupFile
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.config.Bucket),
		Prefix: aws.String(s3Prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			// Skip if object is a directory
			if strings.HasSuffix(*obj.Key, "/") {
				continue
			}

			// Get filename without path
			name := filepath.Base(*obj.Key)

			// Handle nil Size pointer
			var size int64
			if obj.Size != nil {
				size = *obj.Size
			}

			backups = append(backups, BackupFile{
				Name:    name,
				Size:    size,
				ModTime: obj.LastModified.UTC().Format("2006-01-02 15:04:05 UTC"),
			})
		}
	}

	debugLog("Found %d backup files", len(backups))
	return backups, nil
}

// Delete deletes a file from S3 storage
func (s *S3Storage) Delete(remoteName string) error {
	debugLog("Deleting file: %s", remoteName)

	// Create S3 key (path + filename)
	key := filepath.Join(s.config.Path, remoteName)
	key = strings.TrimPrefix(key, "./") // Remove ./ prefix if present

	_, err := s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// Close closes any open connections
func (s *S3Storage) Close() error {
	// No connections to close for S3
	return nil
}
