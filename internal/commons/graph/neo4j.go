package graph

import (
	"context"
	"fmt"
	"github.com/yourusername/astra-backend/internal/commons/util"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neo4jConfig holds the configuration for connecting to Neo4j
type Neo4jConfig struct {
	Uri      string
	Username string
	Password string
	Realm    string // Optional
}

// Neo4jClient wraps the Neo4j driver to provide common utility methods
type Neo4jClient struct {
	driver neo4j.DriverWithContext
}

// NewNeo4jClient creates a new Neo4j client
// Note: For AuraDB, ensure valid neo4j+s:// URI is provided in config
func NewNeo4jClient(config Neo4jConfig) (*Neo4jClient, error) {
	authToken := neo4j.BasicAuth(config.Username, config.Password, config.Realm)

	// AuraDB Recommendations:
	// - MaxConnectionLifetime: 5-50 minutes (Aura LB kills idle connections). We set to 30.
	// - ConnectionLivenessCheckTimeout: Check if connection is alive before pulling from pool.
	driver, err := neo4j.NewDriverWithContext(config.Uri, authToken, func(c *neo4j.Config) {
		c.MaxConnectionLifetime = 30 * time.Minute
		c.ConnectionLivenessCheckTimeout = 2 * time.Minute
		// c.Log = neo4j.ConsoleLogger(neo4j.WarningLevel) // Optional: for debugging
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", util.ErrNeo4jConnectionFailed, err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased for initial cloud handshake
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("%w: %v", util.ErrNeo4jConnectionFailed, err)
	}

	return &Neo4jClient{
		driver: driver,
	}, nil
}

// Close closes the Neo4j driver
func (c *Neo4jClient) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

// ExecuteQuery runs a Cypher query and returns the results as a slice of maps.
// This format is compatible with GraphQL JSON response structures.
//
// Parameters:
//   - ctx: Context for the request
//   - query: The Cypher query string
//   - params: Parameters for the Cypher query
//   - dbName: (Optional) Database name to execute against. Pass "" for default.
//
// Returns:
//   - []map[string]interface{}: A list of records, where each record is a map of keys to values.
//   - error: If the query fails
func (c *Neo4jClient) ExecuteQuery(ctx context.Context, query string, params map[string]interface{}, dbName string) ([]map[string]interface{}, error) {
	sessionConfig := neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead}
	if dbName != "" {
		sessionConfig.DatabaseName = dbName
	}

	session := c.driver.NewSession(ctx, sessionConfig)
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		var records []map[string]interface{}
		for result.Next(ctx) {
			record := result.Record()
			// Convert record to map[string]interface{} for GraphQL compatibility
			// AsValues() returns the map of all keys in the record
			records = append(records, record.AsMap())
		}

		if err := result.Err(); err != nil {
			return nil, err
		}

		return records, nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", util.ErrNeo4jExecutionFailed, err)
	}

	return result.([]map[string]interface{}), nil
}

// ExecuteWriteQuery runs a Cypher query that modifies data.
func (c *Neo4jClient) ExecuteWriteQuery(ctx context.Context, query string, params map[string]interface{}, dbName string) (interface{}, error) {
	sessionConfig := neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite}
	if dbName != "" {
		sessionConfig.DatabaseName = dbName
	}

	session := c.driver.NewSession(ctx, sessionConfig)
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		// Consuming the result to ensure query execution finishes
		summary, err := result.Consume(ctx)
		if err != nil {
			return nil, err
		}
		return summary, nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", util.ErrNeo4jExecutionFailed, err)
	}

	return result, nil
}
