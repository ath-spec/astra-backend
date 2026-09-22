package logger

import (
	"encoding/json"
	"log"
	"time"
	_ "time/tzdata"
)

type serialLogger struct {
	Microservice Microservice
}

func (s *serialLogger) Log(severity severity, message string) {
	indiaLocation, _ := time.LoadLocation("Asia/Kolkata")
	currentTime := time.Now().In(indiaLocation)
	logMessage := LogMessage{
		Severity:     severity,
		Message:      message,
		Microservice: s.Microservice,
		Timestamp:    currentTime,
	}

	// Serialize logMessage to JSON
	logData, err := json.Marshal(logMessage)
	if err != nil {
		log.Println("Error marshaling log message:", err)
		return
	}

	// Print the JSON log message
	log.Println(string(logData))
}
