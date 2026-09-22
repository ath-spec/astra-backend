package kafka

import "github.com/IBM/sarama"

// Creates a new Kafka consumer group.
func CreateSyncProducer(brokers []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	config.Producer.MaxMessageBytes = 15728640

	return sarama.NewSyncProducer(brokers, config)
}
