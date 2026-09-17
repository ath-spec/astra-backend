package event

import (
	"context"
	"github.com/yourusername/astra-backend/internal/commons/connectors"
	"github.com/yourusername/astra-backend/internal/commons/kafka"
	"github.com/yourusername/astra-backend/internal/commons/logger"
	"strings"

	"github.com/IBM/sarama"
)

type Consumer interface {
	HandleEvents(topics []EventTopic, messageChannel chan kafka.Message, handlers map[EventTopic]func([]byte) error)
}

type consumer struct {
	Consumer sarama.ConsumerGroup
	Group    string
}

func NewConsumer(brokers []string, groupId string) Consumer {
	return &consumer{
		Consumer: connectors.CreateSaramaConsumerGroup(brokers, groupId),
		Group:    groupId,
	}
}

func (c *consumer) listen(topics []EventTopic, eventChannel chan kafka.Message) error {
	handler := kafka.ConsumerGroupHandler{
		MessageChannel: eventChannel,
	}

	forever := make(chan bool)

	topicsString := []string{}
	for _, topic := range topics {
		topicsString = append(topicsString, string(topic))
	}

	go c.Consumer.Consume(context.Background(), topicsString, handler)
	logger.Info("waiting for messages [topics, group]: [%s, %s]", "("+strings.Join(topicsString, ", ")+")", c.Group)

	<-forever

	return nil
}

func (c *consumer) HandleEvents(topics []EventTopic, messageChannel chan kafka.Message, handlers map[EventTopic]func([]byte) error) {
	go c.listen(topics, messageChannel)

	for msg := range messageChannel {
		logger.Info("received message [topic, group]: [%s, %s]", msg.Topic, c.Group)

		if handler, ok := handlers[EventTopic(msg.Topic)]; ok {
			go func(msg kafka.Message) {
				if err := handler(msg.Value); err != nil {
					logger.Error("error handling message [topic, group]: [%s, %s], error: %s", msg.Topic, c.Group, err.Error())
				} else {
					logger.Info("message handled [topic, group]: [%s, %s]", msg.Topic, c.Group)
				}
			}(msg)
		}
	}
}
