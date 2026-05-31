package connector

import (
	"testing"
)

// --- AWSInvokeLambdaConnector ---

func TestAWSInvokeLambdaConnector_MissingFunctionName(t *testing.T) {
	c := &AWSInvokeLambdaConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"access_key_id":     "AKID",
			"secret_access_key": "SECRET",
			"region":            "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing function_name")
	}
}

func TestAWSInvokeLambdaConnector_MissingCredential(t *testing.T) {
	c := &AWSInvokeLambdaConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"function_name": "my-function",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAWSInvokeLambdaConnector_MissingAccessKeyID(t *testing.T) {
	c := &AWSInvokeLambdaConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"function_name": "my-function",
		"_credential": map[string]string{
			"secret_access_key": "SECRET",
			"region":            "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing access_key_id")
	}
}

func TestAWSInvokeLambdaConnector_MissingSecretKey(t *testing.T) {
	c := &AWSInvokeLambdaConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"function_name": "my-function",
		"_credential": map[string]string{
			"access_key_id": "AKID",
			"region":        "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing secret_access_key")
	}
}

func TestAWSInvokeLambdaConnector_MissingRegion(t *testing.T) {
	c := &AWSInvokeLambdaConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"function_name": "my-function",
		"_credential": map[string]string{
			"access_key_id":     "AKID",
			"secret_access_key": "SECRET",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing region")
	}
}

// --- AWSSendSQSConnector ---

func TestAWSSendSQSConnector_MissingQueueURL(t *testing.T) {
	c := &AWSSendSQSConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"message_body": "hello",
		"_credential": map[string]string{
			"access_key_id":     "AKID",
			"secret_access_key": "SECRET",
			"region":            "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing queue_url")
	}
}

func TestAWSSendSQSConnector_MissingMessageBody(t *testing.T) {
	c := &AWSSendSQSConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"queue_url": "https://sqs.us-east-1.amazonaws.com/123/myqueue",
		"_credential": map[string]string{
			"access_key_id":     "AKID",
			"secret_access_key": "SECRET",
			"region":            "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing message_body")
	}
}

func TestAWSSendSQSConnector_MissingCredential(t *testing.T) {
	c := &AWSSendSQSConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"queue_url":    "https://sqs.us-east-1.amazonaws.com/123/myqueue",
		"message_body": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

// --- AWSPublishSNSConnector ---

func TestAWSPublishSNSConnector_MissingTopicARN(t *testing.T) {
	c := &AWSPublishSNSConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"message": "hello",
		"_credential": map[string]string{
			"access_key_id":     "AKID",
			"secret_access_key": "SECRET",
			"region":            "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing topic_arn")
	}
}

func TestAWSPublishSNSConnector_MissingMessage(t *testing.T) {
	c := &AWSPublishSNSConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic_arn": "arn:aws:sns:us-east-1:123:mytopic",
		"_credential": map[string]string{
			"access_key_id":     "AKID",
			"secret_access_key": "SECRET",
			"region":            "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestAWSPublishSNSConnector_MissingCredential(t *testing.T) {
	c := &AWSPublishSNSConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic_arn": "arn:aws:sns:us-east-1:123:mytopic",
		"message":   "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

// --- Registry ---

func TestRegistry_AWSConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"aws/invoke_lambda",
		"aws/send_sqs",
		"aws/publish_sns",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
