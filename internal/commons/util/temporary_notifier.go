package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// NotificationJob holds the required data to send a notification.
type NotificationJob struct {
	Phone  string
	Zyid   string
	IP     string
	Action string
	Time   time.Time
}

// jobQueue is a buffered channel that holds pending notifications.
// It acts as an in-memory queue to prevent blocking main execution paths.
var jobQueue = make(chan NotificationJob, 100)

func init() {
	// Start a single background worker to process notifications sequentially.
	go worker()
}

// worker listens on the jobQueue and processes jobs one by one.
func worker() {
	for job := range jobQueue {
		processJob(job)
	}
}

// processJob formats the message and sends it via webhook with exponential backoff retries.
func processJob(job NotificationJob) {
	msg := fmt.Sprintf("🚨 [SECURITY NOTIFICATION] Action: %s | Phone: %s | ZYID: %s | IP: %s | Time: %s", job.Action, job.Phone, job.Zyid, job.IP, job.Time.Format(time.RFC3339))

	// Always log it loudly
	log.Println(msg)

	webhookURL := os.Getenv("TEMPORARY_WEBHOOK_URL")
	if webhookURL == "" {
		return
	}

	payload := map[string]string{
		"content": msg,
	}
	jsonPayload, _ := json.Marshal(payload)

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := sendWebhook(webhookURL, jsonPayload)
		if err == nil {
			return // Success
		}
		
		log.Printf("Webhook notification failed (attempt %d/%d): %v\n", i+1, maxRetries, err)
		
		// Exponential backoff: 1s, 2s, 4s
		if i < maxRetries-1 {
			delay := time.Duration(1<<i) * time.Second
			time.Sleep(delay)
		}
	}
	log.Printf("Webhook notification ultimately failed for job: %v\n", job)
}

func sendWebhook(url string, payload []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned non-success status: %s", resp.Status)
	}

	return nil
}

// NotifyAuthAction queues a notification when a user logs in or out.
// If the queue is full, it drops the webhook notification but still logs to prevent blocking.
func NotifyAuthAction(phone, zyid, ip, action string) {
	job := NotificationJob{
		Phone:  phone,
		Zyid:   zyid,
		IP:     ip,
		Action: action,
		Time:   time.Now(),
	}

	select {
	case jobQueue <- job:
		// Job successfully queued
	default:
		// Queue is full, drop the webhook attempt to avoid blocking the caller
		log.Printf("🚨 [SECURITY NOTIFICATION DROPPED (QUEUE FULL)] Action: %s | Phone: %s\n", action, phone)
	}
}

// GetClientIP extracts the real IP address from the request.
func GetClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}
