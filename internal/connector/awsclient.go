package connector

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// NewAWSConfig builds an aws.Config using the provided credential map and default region.
//
// Credential resolution:
//   - If credential is non-nil with access_key_id + secret_access_key, uses static credentials.
//   - If credential is nil, uses AWS SDK default chain (env → config → IRSA → instance metadata).
//
// Region resolution: credential["region"] > defaultRegion > SDK default (AWS_REGION env).
func NewAWSConfig(ctx context.Context, credential map[string]string, defaultRegion string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error

	if credential != nil {
		accessKey := credential["access_key_id"]
		secretKey := credential["secret_access_key"]
		if accessKey != "" && secretKey != "" {
			sessionToken := credential["session_token"]
			opts = append(opts, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken),
			))
		}
	}

	region := ""
	if credential != nil {
		region = credential["region"]
	}
	if region == "" {
		region = defaultRegion
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}

	if cfg.Region == "" {
		return aws.Config{}, fmt.Errorf("AWS region is required: set via credential, aws.region config, or AWS_REGION env var")
	}

	return cfg, nil
}
