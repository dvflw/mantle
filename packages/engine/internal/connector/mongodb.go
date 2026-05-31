package connector

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const mongoConnectTimeout = 10 * time.Second

// MongoFindConnector queries documents from a MongoDB collection.
type MongoFindConnector struct{}

func (c *MongoFindConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	client, db, coll, err := newMongoClient(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("mongodb/find: %w", err)
	}
	defer client.Disconnect(ctx)

	filter := bsonFromMap(params["filter"])
	opts := options.Find()
	if limit, ok := extractInt(params["limit"]); ok && limit > 0 {
		opts.SetLimit(int64(limit))
	}
	if skip, ok := extractInt(params["skip"]); ok && skip > 0 {
		opts.SetSkip(int64(skip))
	}
	if projection, ok := params["projection"].(map[string]any); ok {
		opts.SetProjection(bson.M(projection))
	}

	cursor, err := client.Database(db).Collection(coll).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb/find: %w", err)
	}
	defer cursor.Close(ctx)

	var documents []any
	if err := cursor.All(ctx, &documents); err != nil {
		return nil, fmt.Errorf("mongodb/find: decoding results: %w", err)
	}
	if documents == nil {
		documents = []any{}
	}

	return map[string]any{
		"documents": documents,
		"count":     len(documents),
	}, nil
}

// MongoAggregateConnector runs an aggregation pipeline on a MongoDB collection.
type MongoAggregateConnector struct{}

func (c *MongoAggregateConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	client, db, coll, err := newMongoClient(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("mongodb/aggregate: %w", err)
	}
	defer client.Disconnect(ctx)

	pipelineRaw, _ := params["pipeline"].([]any)
	if len(pipelineRaw) == 0 {
		return nil, fmt.Errorf("mongodb/aggregate: pipeline is required")
	}

	pipeline := make(bson.A, len(pipelineRaw))
	for i, stage := range pipelineRaw {
		if m, ok := stage.(map[string]any); ok {
			pipeline[i] = bson.M(m)
		} else {
			pipeline[i] = stage
		}
	}

	cursor, err := client.Database(db).Collection(coll).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongodb/aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	var documents []any
	if err := cursor.All(ctx, &documents); err != nil {
		return nil, fmt.Errorf("mongodb/aggregate: decoding results: %w", err)
	}
	if documents == nil {
		documents = []any{}
	}

	return map[string]any{
		"documents": documents,
		"count":     len(documents),
	}, nil
}

// newMongoClient connects to MongoDB and returns the client plus target db/collection names.
// Credential: {uri: "mongodb://...", database: "mydb", collection: "mycoll"}
func newMongoClient(ctx context.Context, params map[string]any) (*mongo.Client, string, string, error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return nil, "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var uri, credDB string
	switch cred := raw.(type) {
	case map[string]string:
		uri = cred["uri"]
		credDB = cred["database"]
	case map[string]any:
		uri, _ = cred["uri"].(string)
		credDB, _ = cred["database"].(string)
	default:
		return nil, "", "", fmt.Errorf("credential is required")
	}
	if uri == "" {
		return nil, "", "", fmt.Errorf("credential must contain a 'uri' field")
	}

	db, _ := params["database"].(string)
	if db == "" {
		db = credDB
	}
	if db == "" {
		return nil, "", "", fmt.Errorf("database is required (set in params or credential)")
	}

	coll, _ := params["collection"].(string)
	if coll == "" {
		return nil, "", "", fmt.Errorf("collection is required")
	}

	connectCtx, cancel := context.WithTimeout(ctx, mongoConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, "", "", fmt.Errorf("connecting to MongoDB: %w", err)
	}
	if err := client.Ping(connectCtx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, "", "", fmt.Errorf("pinging MongoDB: %w", err)
	}
	return client, db, coll, nil
}

// bsonFromMap converts a map[string]any filter param into a bson.D, or returns
// an empty bson.D (match-all) if the value is nil or not a map.
func bsonFromMap(v any) bson.D {
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		return bson.D(func() []bson.E {
			out := make([]bson.E, 0, len(m))
			for k, val := range m {
				out = append(out, bson.E{Key: k, Value: val})
			}
			return out
		}())
	}
	return bson.D{}
}
