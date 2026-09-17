package event

import (
	"encoding/json"
)

// NotificationPayload represents the structure of the notification event
type NotificationPayload struct {
	EventID     string                 `json:"event_id"`
	Zyid        string                 `json:"zyid"`
	Category    string                 `json:"category"`
	Priority    string                 `json:"priority"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	ActionURL   string                 `json:"action_url,omitempty"`
	ImageURL    string                 `json:"image_url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Timestamp   int64                  `json:"timestamp"`
	IsPush      bool                   `json:"is_push"`
	IsInApp     bool                   `json:"is_in_app"`
	TargetUsers []string               `json:"target_users,omitempty"`
}

func (p *producer) ProduceNotification(payload NotificationPayload) error {
	j, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Using PERSONAL_NOTIFICATION topic as seen in producer.go constants
	return p.produce(j, PERSONAL_NOTIFICATION)
}
