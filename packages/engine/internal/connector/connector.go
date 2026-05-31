package connector

import (
	"context"
	"fmt"
)

// DefaultMaxResponseBytes is the maximum number of bytes read from any HTTP
// response body across all connectors. This prevents OOM from large or
// malicious responses. 10 MB.
const DefaultMaxResponseBytes = 10 * 1024 * 1024

// Connector executes an action with the given parameters and returns output data.
type Connector interface {
	Execute(ctx context.Context, params map[string]any) (map[string]any, error)
}

// Registry maps action names to connector implementations.
type Registry struct {
	connectors map[string]Connector
}

// NewRegistry creates a registry with the built-in connectors registered.
func NewRegistry() *Registry {
	r := &Registry{
		connectors: make(map[string]Connector),
	}
	r.Register("http/request", &HTTPConnector{})
	r.Register("ai/completion", &AIConnector{})
	r.Register("slack/send", &SlackSendConnector{})
	r.Register("slack/history", &SlackHistoryConnector{})
	r.Register("postgres/query", &PostgresQueryConnector{})
	r.Register("email/send", &EmailSendConnector{})
	r.Register("email/receive", &EmailReceiveConnector{})
	r.Register("email/move", &EmailMoveConnector{})
	r.Register("email/delete", &EmailDeleteConnector{})
	r.Register("email/flag", &EmailFlagConnector{})
	r.Register("s3/put", &S3PutConnector{})
	r.Register("s3/get", &S3GetConnector{})
	r.Register("s3/list", &S3ListConnector{})
	r.Register("docker/run", &DockerRunConnector{})
	r.Register("browser/run", &BrowserRunConnector{})
	r.Register("github/create_issue", &GitHubCreateIssueConnector{})
	r.Register("github/dispatch", &GitHubDispatchConnector{})
	r.Register("linear/create_issue", &LinearCreateIssueConnector{})
	r.Register("linear/search", &LinearSearchConnector{})
	r.Register("notion/create_page", &NotionCreatePageConnector{})
	r.Register("notion/query_database", &NotionQueryDatabaseConnector{})
	r.Register("airtable/list", &AirtableListConnector{})
	r.Register("airtable/create_record", &AirtableCreateRecordConnector{})
	r.Register("pagerduty/create_incident", &PagerDutyCreateIncidentConnector{})
	r.Register("pagerduty/resolve", &PagerDutyResolveConnector{})
	r.Register("twilio/sms", &TwilioSMSConnector{})
	r.Register("twilio/call", &TwilioCallConnector{})
	r.Register("asana/create_task", &AsanaCreateTaskConnector{})
	r.Register("asana/search", &AsanaSearchConnector{})
	r.Register("discord/send", &DiscordSendConnector{})
	r.Register("discord/embed", &DiscordEmbedConnector{})
	r.Register("elasticsearch/search", &ElasticsearchSearchConnector{})
	r.Register("elasticsearch/index", &ElasticsearchIndexConnector{})
	r.Register("datadog/submit_event", &DatadogSubmitEventConnector{})
	r.Register("datadog/query_metrics", &DatadogQueryMetricsConnector{})
	r.Register("redis/get", &RedisGetConnector{})
	r.Register("redis/set", &RedisSetConnector{})
	r.Register("redis/publish", &RedisPublishConnector{})
	r.Register("mongodb/find", &MongoFindConnector{})
	r.Register("mongodb/aggregate", &MongoAggregateConnector{})
	r.Register("stripe/create_charge", &StripeCreateChargeConnector{})
	r.Register("stripe/create_customer", &StripeCreateCustomerConnector{})
	r.Register("stripe/create_refund", &StripeCreateRefundConnector{})
	r.Register("okta/list_users", &OktaListUsersConnector{})
	r.Register("okta/create_user", &OktaCreateUserConnector{})
	r.Register("quickbooks/create_invoice", &QuickBooksCreateInvoiceConnector{})
	r.Register("quickbooks/list_invoices", &QuickBooksListInvoicesConnector{})
	r.Register("onedrive/upload", &OneDriveUploadConnector{})
	r.Register("sharepoint/list_items", &SharePointListItemsConnector{})
	r.Register("rabbitmq/publish", &RabbitMQPublishConnector{})
	r.Register("rabbitmq/consume", &RabbitMQConsumeConnector{})
	r.Register("shopify/list_orders", &ShopifyListOrdersConnector{})
	r.Register("shopify/list_products", &ShopifyListProductsConnector{})
	r.Register("shopify/create_order", &ShopifyCreateOrderConnector{})
	r.Register("mailchimp/list_members", &MailchimpListMembersConnector{})
	r.Register("mailchimp/add_member", &MailchimpAddMemberConnector{})
	return r
}

// Register adds a connector for the given action name.
func (r *Registry) Register(action string, c Connector) {
	r.connectors[action] = c
}

// Get returns the connector for the given action, or an error if not found.
func (r *Registry) Get(action string) (Connector, error) {
	c, ok := r.connectors[action]
	if !ok {
		return nil, fmt.Errorf("unknown action %q", action)
	}
	return c, nil
}
