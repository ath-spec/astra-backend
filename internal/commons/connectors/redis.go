package connectors

import (
	"github.com/yourusername/astra-backend/internal/commons/logger"
	"github.com/yourusername/astra-backend/internal/commons/redis"
	"strings"
	"time"
)

// Infinite iterator that returns the Redis session
func CreateRedisClusterConnection(host []string) redis.Client {
	count := 0
	for {
		client, err := redis.CreateRedisClusterSession(host)
		if err != nil {
			count++
		} else {
			logger.Info("connected to redis!")
			return client
		}
		if count == 5 {
			logger.Error("unable to connect to redis: %s", err)
			logger.Info("retying in 5 seconds...")
			time.Sleep(time.Second * 5)
			count = 0
		}
	}
}

func CreateRedisConnection(host []string) redis.Client {
	count := 0
	for {
		client, err := redis.CreateRedisSession(host[0])
		if err != nil {
			count++
		} else {
			logger.Info("connected to redis!")
			return client
		}
		if count == 5 {
			logger.Error("unable to connect to redis: %s", err)
			logger.Info("retying in 5 seconds...")
			time.Sleep(time.Second * 5)
			count = 0
		}
	}
}

func CreateRedisConnectionFromURL(redisURL string) redis.Client {
	count := 0
	// For logging, sanitize the URL (hide password)
	sanitizedURL := redisURL
	if atIndex := strings.LastIndex(redisURL, "@"); atIndex != -1 {
		if colonIndex := strings.Index(redisURL, ":"); colonIndex != -1 && colonIndex < atIndex {
			// Find the second colon (after redis://)
			if secondColon := strings.Index(redisURL[colonIndex+3:], ":"); secondColon != -1 && secondColon+colonIndex+3 < atIndex {
				sanitizedURL = redisURL[:secondColon+colonIndex+4] + "****" + redisURL[atIndex:]
			}
		}
	}

	for {
		client, err := redis.CreateRedisSessionFromURL(redisURL)
		if err != nil {
			count++
		} else {
			logger.Info("connected to redis at %s", sanitizedURL)
			return client
		}
		if count == 5 {
			logger.Error("unable to connect to redis at %s: %s", sanitizedURL, err)
			logger.Info("retying in 5 seconds...")
			time.Sleep(time.Second * 5)
			count = 0
		}
	}
}
