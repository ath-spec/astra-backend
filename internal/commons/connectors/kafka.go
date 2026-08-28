package connectors

// Kafka connector — PROVISION FOR LATER, not yet wired.
//
// Astra processes everything inline today. A broker is warranted when work must
// survive a crash and be retried independently of the request:
//
//   - AA / bank-statement ingestion and enrichment
//   - portfolio snapshot + DNA recomputation fan-out
//   - outbound notifications / webhooks
//   - an audit event stream
//
// On AWS the first stop is usually SQS (simpler, already in the infra ask) — reach
// for MSK/Kafka only when you need ordered partitioned replay or multiple
// independent consumer groups over the same log. If it stays SQS, this file is
// replaced by an sqs.go connector with the same retry shape.
//
// To activate: `go get github.com/IBM/sarama`, uncomment below, and construct the
// producer/consumer in cmd/api/main.go. Config knob: KAFKA_BROKERS (comma list).
// The retry/backoff mirrors CreatePostgresPool.
//
// Ported from z-backend server/common/kafka + server/common/connectors/sarama-*.go.

/*
import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
)

func baseConfig() *sarama.Config {
	c := sarama.NewConfig()
	c.Producer.RequiredAcks = sarama.WaitForAll
	c.Producer.Return.Successes = true
	c.Producer.MaxMessageBytes = 15 << 20
	c.Consumer.Return.Errors = true
	return c
}

// CreateSyncProducer returns a Ping-equivalent (dial-verified) sync producer,
// retrying a bounded number of times (same policy as CreatePostgresPool).
func CreateSyncProducer(ctx context.Context, brokers []string) (sarama.SyncProducer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers list is empty")
	}
	var lastErr error
	for attempt := 1; attempt <= connectRetries; attempt++ {
		p, err := sarama.NewSyncProducer(brokers, baseConfig())
		if err == nil {
			slog.Info("connected to kafka (producer)", "brokers", brokers)
			return p, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, fmt.Errorf("unable to connect to kafka: %w", ctx.Err())
		}
		if attempt < connectRetries {
			slog.Warn("kafka not ready, retrying", "attempt", attempt, "of", connectRetries, "error", err)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("unable to connect to kafka: %w", ctx.Err())
			case <-time.After(connectRetryDelay):
			}
		}
	}
	return nil, fmt.Errorf("unable to connect to kafka after %d attempts: %w", connectRetries, lastErr)
}

// CreateConsumerGroup returns a consumer group with the same bounded retry.
func CreateConsumerGroup(ctx context.Context, brokers []string, groupID string) (sarama.ConsumerGroup, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers list is empty")
	}
	cfg := baseConfig()
	cfg.ClientID = fmt.Sprintf("%s-%d", groupID, time.Now().UnixMilli())

	var lastErr error
	for attempt := 1; attempt <= connectRetries; attempt++ {
		cg, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
		if err == nil {
			slog.Info("connected to kafka (consumer group)", "brokers", brokers, "group", groupID)
			return cg, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, fmt.Errorf("unable to connect to kafka: %w", ctx.Err())
		}
		if attempt < connectRetries {
			slog.Warn("kafka not ready, retrying", "attempt", attempt, "of", connectRetries, "error", err)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("unable to connect to kafka: %w", ctx.Err())
			case <-time.After(connectRetryDelay):
			}
		}
	}
	return nil, fmt.Errorf("unable to connect to kafka after %d attempts: %w", connectRetries, lastErr)
}
*/
