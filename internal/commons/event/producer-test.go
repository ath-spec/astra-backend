package event

import (
	"time"

	"github.com/google/uuid"
)

type producerTest struct{}

// ProduceProfileDeleted implements Producer.
func (p *producerTest) ProduceProfileDeleted(id uuid.UUID, vin string, phone string) error {
	return nil
}

// ProduceCoinsEarned implements Producer.
func (p *producerTest) ProduceCoinsEarned(zyid string, featureName string, taskName string, category string, calories ...string) error {
	return nil
}

// ProduceCoinsEarneFlink implements Producer.
func (p *producerTest) ProduceCoinsEarnedFlink(zyid string, featureName string, taskName string, category string, calories ...string) error {
	return nil
}

// ProducePersonalNotification implements Producer.
func (p *producerTest) ProducePersonalNotification(id, title, body string, time time.Time) error {
	return nil
}

// ProduceRole implements Producer.
func (p *producerTest) ProduceRole(id uuid.UUID, userrole string) error {
	return nil
}

// ProduceUser implements Producer.
func (p *producerTest) ProduceUser(phone string, id uuid.UUID) error {
	return nil
}

// ProduceServerNotification implements Producer.
func (*producerTest) ProduceServerNotification(zyid uuid.UUID, category string, title string, body string) error {
	return nil
}

// ProducePersonalConditionalNotification implements Producer.
func (*producerTest) ProducePersonalConditionalNotification(ids []string, title string, body string) error {
	return nil
}

// ProduceEmail implements Producer.
func (*producerTest) ProduceEmail(email string, id uuid.UUID) error {
	return nil
}

// ProduceUserBike implements Producer.
func (*producerTest) ProduceUserBike(id uuid.UUID, vin string, model string, pass string) error {
	return nil
}

// ProduceProfileAdded implements Producer.
func (*producerTest) ProduceProfileAdded(id uuid.UUID, username string, phone string, userrole string, profilepictureextension string, profilepicture string) error {
	return nil
}

// ProduceNotification implements Producer.
func (p *producerTest) ProduceNotification(payload NotificationPayload) error {
	return nil
}

func NewProducerTest() Producer {
	return &producerTest{}
}
