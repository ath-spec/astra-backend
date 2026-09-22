package connectors

import (
	"github.com/yourusername/astra-backend/internal/commons/kafka"
	"github.com/yourusername/astra-backend/internal/commons/logger"
	"time"

	"github.com/IBM/sarama"
)

// Infinite iterator that returns the Kafka session
func CreateSaramaSyncProducer(brokers []string) sarama.SyncProducer {
	count := 0
	for {
		producer, err := kafka.CreateSyncProducer(brokers)
		if err != nil {
			count++
		} else {
			logger.Info("kafka async producer created!")
			return producer
		}
		if count == 5 {
			logger.Error("unable to connect to kafka: %s", err)
			logger.Info("retrying in 5 seconds...")
			count = 0
			time.Sleep(time.Second * 5)
		}
	}
}
