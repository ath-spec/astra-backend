package sms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"github.com/yourusername/astra-backend/internal/commons/logger"
	"github.com/yourusername/astra-backend/internal/commons/util"
)

type Sender interface {
	SendPointSms(authKey, flowID, mobile, senderID string, variables map[string]string) error
	Send2FactorSms(authKey, flowID, mobile string, otp int) error
}

type sender struct{}

func NewSmsSender() Sender {
	return &sender{}
}

func (s *sender) SendPointSms(authKey, flowID, mobile, senderID string, variables map[string]string) error {
	// flatten all into root
	payload := map[string]interface{}{
		"flow_id": flowID,
		"sender":  senderID,
		"mobiles": mobile,
	}

	// add all variables like var1, var2 to the root
	for k, v := range variables {
		payload[k] = v
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to marshal MSG91 Flow payload: %v", err)
		return util.ErrInternal
	}

	logger.Debug("MSG91 Flow Request Payload: %s", string(data))

	req, err := http.NewRequest("POST", "https://api.msg91.com/api/v5/flow/", bytes.NewBuffer(data))
	if err != nil {
		logger.Error("Failed to create MSG91 request: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authkey", authKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to send MSG91 request: %v", err)
		return util.ErrSmsUnableToSend
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	logger.Debug("MSG91 Response: %s", string(body))

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		logger.Error("MSG91 failed with status: %d", resp.StatusCode)
		return util.ErrSmsUnableToSend
	}

	return nil
}

func (s *sender) Send2FactorSms(authKey, flowID, mobile string, otp int) error {
	variables := map[string]string{
		"var":  fmt.Sprintf("%d", otp), // For templates using ##var##
		"var1": fmt.Sprintf("%d", otp), // For templates using ##var1##
	}

	// Sender ID should be set as per your Flow setup (e.g., "SMSIND")
	return s.SendPointSms(authKey, flowID, mobile, "SMSIND", variables)
}
