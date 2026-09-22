package event

import (
	"github.com/yourusername/astra-backend/internal/commons/connectors"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
)

type Producer interface {
	ProduceUserBike(id uuid.UUID, vin, model, pass string) error
	// ProduceUserId(id uuid.UUID) error
	ProduceProfileAdded(id uuid.UUID, username string, phone string, userrole string, profilepictureextension string, profilepicture string) error
	ProduceProfileDeleted(id uuid.UUID, vin string, phone string) error
	ProduceUser(phone string, id uuid.UUID) error
	ProduceRole(id uuid.UUID, userrole string) error
	ProduceEmail(email string, id uuid.UUID) error
	ProduceNotification(payload NotificationPayload) error
}

type EventTopic string

const (
	USERBIKE EventTopic = "event-userbike"
	// USERID                            EventTopic = "event-user-id"
	USER_PROFILE_ADDED                EventTopic = "event-user-profile-added"
	USER_PROFILE_DELETED              EventTopic = "event-user-profile-deleted"
	USER                              EventTopic = "event-user"
	ROLE                              EventTopic = "event-role"
	EMAIL                             EventTopic = "event-email"
	PERSONAL_NOTIFICATION             EventTopic = "event-personal-notification"
	PERSONAL_CONDITIONAL_NOTIFICATION EventTopic = "event-personal-conditional-notification"
	SERVER_NOTIFICATION               EventTopic = "event-server-notification"
	USER_LOCATIONS                    EventTopic = "event-user-locations"
	TRIP_AGGREGATE                    EventTopic = "event-trip-aggregate"
	ACCESSORY_LOG                     EventTopic = "event-accessory-logs"
	SERVICE_NOTIFICATION              EventTopic = "service-notification"
	COINS_EARNED                      EventTopic = "event-user-activity-point"
	COINS_EARNED_FLINK                EventTopic = "event-user-activity-point-flink"
)

type producer struct {
	Producer sarama.SyncProducer
}

func NewProducer(brokers []string) Producer {
	if len(brokers) == 0 {
		println("[MockProducer] Kafka brokers empty, using MockProducer")
		return &MockProducer{}
	}
	return &producer{
		Producer: connectors.CreateSaramaSyncProducer(brokers),
	}
}

type MockProducer struct{}

func (m *MockProducer) ProduceUserBike(id uuid.UUID, vin, model, pass string) error {
	println("[MockProducer] ProduceUserBike")
	return nil
}
func (m *MockProducer) ProduceProfileAdded(id uuid.UUID, username string, phone string, userrole string, profilepictureextension string, profilepicture string) error {
	println("[MockProducer] ProduceProfileAdded")
	return nil
}
func (m *MockProducer) ProduceProfileDeleted(id uuid.UUID, vin string, phone string) error {
	println("[MockProducer] ProduceProfileDeleted")
	return nil
}
func (m *MockProducer) ProduceUser(phone string, id uuid.UUID) error {
	println("[MockProducer] ProduceUser")
	return nil
}
func (m *MockProducer) ProduceRole(id uuid.UUID, userrole string) error {
	println("[MockProducer] ProduceRole")
	return nil
}
func (m *MockProducer) ProduceEmail(email string, id uuid.UUID) error {
	println("[MockProducer] ProduceEmail")
	return nil
}
func (m *MockProducer) ProduceNotification(payload NotificationPayload) error {
	println("[MockProducer] ProduceNotification: " + payload.Title)
	return nil
}

func (p *producer) produce(message []byte, route EventTopic) error {
	_, _, err := p.Producer.SendMessage(&sarama.ProducerMessage{
		Topic: string(route),
		Value: sarama.StringEncoder(message),
	})

	return err
}
