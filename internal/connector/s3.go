package connector

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// newS3Client builds an S3 client from the _credential map in params.
// It extracts and deletes the credential key from params.
// Supported credential fields: access_key, secret_key, region, endpoint.
func newS3Client(params map[string]any) (*s3.Client, error) {
	cred, ok := params["_credential"].(map[string]string)
	if !ok {
		return nil, fmt.Errorf("s3: _credential is required (need access_key and secret_key)")
	}
	delete(params, "_credential")

	accessKey := cred["access_key"]
	secretKey := cred["secret_key"]
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("s3: access_key and secret_key are required in credential")
	}

	region := cred["region"]
	if region == "" {
		region = "us-east-1"
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = region
			o.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		},
	}

	if endpoint := cred["endpoint"]; endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Required for most S3-compatible services (MinIO, etc.)
		})
	}

	client := s3.New(s3.Options{}, opts...)
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

	client, err := newS3Client(params)
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

	client, err := newS3Client(params)
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

	data, err := io.ReadAll(result.Body)
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

	client, err := newS3Client(params)
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

