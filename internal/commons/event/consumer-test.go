package event

import (
	"github.com/yourusername/astra-backend/internal/commons/kafka"
)

type consumerTest struct{}

func NewConsumerTest() Consumer {
	return &consumerTest{}
}

func (c *consumerTest) HandleEvents(topics []EventTopic, messageChannel chan kafka.Message, handlers map[EventTopic]func([]byte) error) {
}
