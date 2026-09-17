package kafka

import (
	"fmt"
	"time"

	"github.com/IBM/sarama"
)

// Creates a new Kafka consumer group.
func CreateConsumerGroup(brokers []string, group string) (sarama.ConsumerGroup, error) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.ClientID = fmt.Sprintf("%s-%d", group, time.Now().UnixMilli())
	//sarama.Logger = log.New(os.Stdout, "[Sarama-Kafka] ", log.LstdFlags)

	return sarama.NewConsumerGroup(brokers, group, config)
}

type Message struct {
	Topic string
	Key   []byte
	Value []byte
}

type ConsumerGroupHandler struct {
	MessageChannel chan Message
}

func (ConsumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h ConsumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		h.MessageChannel <- Message{
			Topic: message.Topic,
			Key:   message.Key,
			Value: message.Value,
		}
		session.MarkMessage(message, "")
		session.Commit()
	}
	return nil
}
