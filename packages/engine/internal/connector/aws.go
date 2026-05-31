package connector

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// extractAWSCredential pulls access_key_id, secret_access_key, and region
// from the _credential param. Both map[string]string and map[string]any shapes
// are supported. Deletes _credential from params.
func extractAWSCredential(params map[string]any) (accessKeyID, secretAccessKey, region string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var cred map[string]string
	switch v := raw.(type) {
	case map[string]string:
		cred = v
	case map[string]any:
		cred = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				cred[k] = s
			}
		}
	default:
		return "", "", "", fmt.Errorf("credential is required")
	}

	accessKeyID = cred["access_key_id"]
	secretAccessKey = cred["secret_access_key"]
	region = cred["region"]

	if accessKeyID == "" {
		return "", "", "", fmt.Errorf("credential must contain an 'access_key_id' field")
	}
	if secretAccessKey == "" {
		return "", "", "", fmt.Errorf("credential must contain a 'secret_access_key' field")
	}
	if region == "" {
		return "", "", "", fmt.Errorf("credential must contain a 'region' field")
	}
	return accessKeyID, secretAccessKey, region, nil
}

// newAWSConfig builds an aws.Config with static credentials and region.
func newAWSConfig(ctx context.Context, accessKeyID, secretAccessKey, region string) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}
	return cfg, nil
}

// AWSInvokeLambdaConnector invokes an AWS Lambda function.
// Params: function_name (required), payload (optional JSON string).
// Output: {"status_code": N, "payload": "...", "function_error": "..."}
type AWSInvokeLambdaConnector struct{}

func (c *AWSInvokeLambdaConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	functionName, _ := params["function_name"].(string)
	if functionName == "" {
		return nil, fmt.Errorf("aws/invoke_lambda: function_name is required")
	}

	payload, _ := params["payload"].(string)

	accessKeyID, secretAccessKey, region, err := extractAWSCredential(params)
	if err != nil {
		return nil, fmt.Errorf("aws/invoke_lambda: %w", err)
	}

	cfg, err := newAWSConfig(ctx, accessKeyID, secretAccessKey, region)
	if err != nil {
		return nil, fmt.Errorf("aws/invoke_lambda: %w", err)
	}

	client := lambda.NewFromConfig(cfg)
	input := &lambda.InvokeInput{
		FunctionName: aws.String(functionName),
	}
	if payload != "" {
		input.Payload = []byte(payload)
	}

	resp, err := client.Invoke(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("aws/invoke_lambda: %w", err)
	}

	result := map[string]any{
		"status_code": int(resp.StatusCode),
		"payload":     string(resp.Payload),
	}
	if resp.FunctionError != nil {
		result["function_error"] = *resp.FunctionError
	} else {
		result["function_error"] = nil
	}
	return result, nil
}

// AWSSendSQSConnector sends a message to an SQS queue.
// Params: queue_url (required), message_body (required),
//
//	delay_seconds (optional int32), message_group_id (optional).
//
// Output: {"message_id": "..."}
type AWSSendSQSConnector struct{}

func (c *AWSSendSQSConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	queueURL, _ := params["queue_url"].(string)
	if queueURL == "" {
		return nil, fmt.Errorf("aws/send_sqs: queue_url is required")
	}

	messageBody, _ := params["message_body"].(string)
	if messageBody == "" {
		return nil, fmt.Errorf("aws/send_sqs: message_body is required")
	}

	accessKeyID, secretAccessKey, region, err := extractAWSCredential(params)
	if err != nil {
		return nil, fmt.Errorf("aws/send_sqs: %w", err)
	}

	cfg, err := newAWSConfig(ctx, accessKeyID, secretAccessKey, region)
	if err != nil {
		return nil, fmt.Errorf("aws/send_sqs: %w", err)
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(messageBody),
	}

	if ds, ok := extractInt(params["delay_seconds"]); ok && ds > 0 {
		input.DelaySeconds = int32(ds) // #nosec G115 -- delay_seconds valid range 0-900, safe narrowing
	}
	if mgid, _ := params["message_group_id"].(string); mgid != "" {
		input.MessageGroupId = aws.String(mgid)
	}

	client := sqs.NewFromConfig(cfg)
	result, err := client.SendMessage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("aws/send_sqs: %w", err)
	}

	return map[string]any{
		"message_id": aws.ToString(result.MessageId),
	}, nil
}

// AWSPublishSNSConnector publishes a message to an SNS topic.
// Params: topic_arn (required), message (required),
//
//	subject (optional), message_structure (optional).
//
// Output: {"message_id": "..."}
type AWSPublishSNSConnector struct{}

func (c *AWSPublishSNSConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	topicARN, _ := params["topic_arn"].(string)
	if topicARN == "" {
		return nil, fmt.Errorf("aws/publish_sns: topic_arn is required")
	}

	message, _ := params["message"].(string)
	if message == "" {
		return nil, fmt.Errorf("aws/publish_sns: message is required")
	}

	accessKeyID, secretAccessKey, region, err := extractAWSCredential(params)
	if err != nil {
		return nil, fmt.Errorf("aws/publish_sns: %w", err)
	}

	cfg, err := newAWSConfig(ctx, accessKeyID, secretAccessKey, region)
	if err != nil {
		return nil, fmt.Errorf("aws/publish_sns: %w", err)
	}

	input := &sns.PublishInput{
		TopicArn: aws.String(topicARN),
		Message:  aws.String(message),
	}

	if subject, _ := params["subject"].(string); subject != "" {
		input.Subject = aws.String(subject)
	}
	if ms, _ := params["message_structure"].(string); ms != "" {
		input.MessageStructure = aws.String(ms)
	}

	client := sns.NewFromConfig(cfg)
	result, err := client.Publish(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("aws/publish_sns: %w", err)
	}

	return map[string]any{
		"message_id": aws.ToString(result.MessageId),
	}, nil
}
