package connector

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// newS3Client builds an S3 client from the _credential map in params.
// It extracts and deletes the credential key from params.
// Supported credential fields: access_key_id (or legacy access_key), secret_access_key (or legacy secret_key), region, endpoint.
// If _credential is absent or nil, credentials are sourced from the AWS SDK default chain.
func newS3Client(ctx context.Context, params map[string]any, defaultRegion string) (*s3.Client, error) {
	var cred map[string]string
	if raw, ok := params["_credential"]; ok {
		cred, _ = raw.(map[string]string)
		delete(params, "_credential")
	}

	// Normalize legacy field names for backward compatibility.
	if cred != nil {
		if cred["access_key_id"] == "" && cred["access_key"] != "" {
			cred["access_key_id"] = cred["access_key"]
		}
		if cred["secret_access_key"] == "" && cred["secret_key"] != "" {
			cred["secret_access_key"] = cred["secret_key"]
		}
	}

	// Capture endpoint before calling NewAWSConfig (endpoint is not part of aws.Config).
	var endpoint string
	if cred != nil {
		endpoint = cred["endpoint"]
	}

	awsCfg, err := NewAWSConfig(ctx, cred, defaultRegion)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	var opts []func(*s3.Options)
	if endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Required for most S3-compatible services (MinIO, etc.)
		})
	}

	client := s3.NewFromConfig(awsCfg, opts...)
	return client, nil
}

// S3PutConnector uploads an object to S3.
// Params: bucket (required), key (required), content (required string), content_type (optional).
// Output: {"bucket": "...", "key": "...", "size": N}
type S3PutConnector struct{}

func (c *S3PutConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	bucket, _ := params["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("s3/put: bucket is required")
	}

	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("s3/put: key is required")
	}

	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("s3/put: content is required")
	}

	client, err := newS3Client(ctx, params, "")
	if err != nil {
		return nil, err
	}

	contentType := "application/octet-stream"
	if ct, ok := params["content_type"].(string); ok && ct != "" {
		contentType = ct
	}

	body := strings.NewReader(content)
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}

	_, err = client.PutObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("s3/put: %w", err)
	}

	return map[string]any{
		"bucket": bucket,
		"key":    key,
		"size":   int64(len(content)),
	}, nil
}

// S3GetConnector downloads an object from S3.
// Params: bucket (required), key (required).
// Output: {"bucket": "...", "key": "...", "content": "...", "size": N, "content_type": "..."}
type S3GetConnector struct{}

func (c *S3GetConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	bucket, _ := params["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("s3/get: bucket is required")
	}

	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("s3/get: key is required")
	}

	client, err := newS3Client(ctx, params, "")
	if err != nil {
		return nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("s3/get: %w", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(io.LimitReader(result.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("s3/get: reading object body: %w", err)
	}

	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	return map[string]any{
		"bucket":       bucket,
		"key":          key,
		"content":      string(data),
		"size":         int64(len(data)),
		"content_type": contentType,
	}, nil
}

// S3ListConnector lists objects in an S3 bucket.
// Params: bucket (required), prefix (optional).
// Output: {"bucket": "...", "objects": [{"key": "...", "size": N, "last_modified": "..."}]}
type S3ListConnector struct{}

func (c *S3ListConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	bucket, _ := params["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("s3/list: bucket is required")
	}

	client, err := newS3Client(ctx, params, "")
	if err != nil {
		return nil, err
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	if prefix, ok := params["prefix"].(string); ok && prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var objects []map[string]any

	paginator := s3.NewListObjectsV2Paginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3/list: %w", err)
		}
		for _, obj := range page.Contents {
			entry := map[string]any{
				"key":  aws.ToString(obj.Key),
				"size": aws.ToInt64(obj.Size),
			}
			if obj.LastModified != nil {
				entry["last_modified"] = obj.LastModified.UTC().Format("2006-01-02T15:04:05Z")
			}
			objects = append(objects, entry)
		}
	}

	if objects == nil {
		objects = []map[string]any{}
	}

	return map[string]any{
		"bucket":  bucket,
		"objects": objects,
	}, nil
}

