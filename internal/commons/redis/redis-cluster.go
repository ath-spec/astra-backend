package redis

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
	Expire(ctx context.Context, key string, expiration time.Duration) (bool, error)
	RPush(ctx context.Context, key string, values ...interface{}) (int64, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	LTrim(ctx context.Context, key string, start, stop int64) (string, error)
	LLen(ctx context.Context, key string) (int64, error)
	ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) *redis.ZSliceCmd
	ZScore(ctx context.Context, key string, member string) (float64, error)
	ZRevRank(ctx context.Context, key string, member string) (int64, error)
	HSet(ctx context.Context, key string, values ...interface{}) (int64, error)
	HGet(ctx context.Context, key string, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, values ...interface{}) (int64, error)
	ZCard(ctx context.Context, key string) *redis.IntCmd
	Incr(ctx context.Context, key string) (int64, error)
	Exists(ctx context.Context, keys ...string) (int64, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	Close() error
}

type ClusterClient struct {
	Host    []string
	Session *redis.ClusterClient
}

func CreateRedisClusterSession(host []string) (Client, error) {
	redisClient := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:     host,
		TLSConfig: &tls.Config{},
	})
	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		return nil, err
	}
	return &ClusterClient{
		Host:    host,
		Session: redisClient,
	}, nil
}

func (c *ClusterClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.Session.Set(ctx, key, value, expiration).Err()
}

func (c *ClusterClient) Get(ctx context.Context, key string) (string, error) {
	return c.Session.Get(ctx, key).Result()
}

func (c *ClusterClient) Del(ctx context.Context, key string) error {
	return c.Session.Del(ctx, key).Err()
}

func (c *ClusterClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.Session.Expire(ctx, key, expiration).Result()
}

func (c *ClusterClient) RPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.Session.RPush(ctx, key, values...).Result()
}

func (c *ClusterClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.Session.LRange(ctx, key, start, stop).Result()
}

func (c *ClusterClient) LTrim(ctx context.Context, key string, start, stop int64) (string, error) {
	return c.Session.LTrim(ctx, key, start, stop).Result()
}

func (c *ClusterClient) LLen(ctx context.Context, key string) (int64, error) {
	return c.Session.LLen(ctx, key).Result()
}

func (c *ClusterClient) Close() error {
	return c.Session.Close()
}

func (c *ClusterClient) ZRevRangeWithScores(ctx context.Context, key string, start int64, stop int64) *redis.ZSliceCmd {
	return c.Session.ZRevRangeWithScores(ctx, key, start, stop)
}

func (c *ClusterClient) ZRevRank(ctx context.Context, key string, member string) (int64, error) {
	return c.Session.ZRevRank(ctx, key, member).Result()
}

func (c *ClusterClient) ZScore(ctx context.Context, key string, member string) (float64, error) {
	return c.Session.ZScore(ctx, key, member).Result()
}

func (c *ClusterClient) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.Session.HSet(ctx, key, values).Result()
}

func (c *ClusterClient) HDel(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.Session.HDel(ctx, key).Result()
}

func (c *ClusterClient) HGet(ctx context.Context, key string, field string) (string, error) {
	return c.Session.HGet(ctx, key, field).Result()
}

func (c *ClusterClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.Session.HGetAll(ctx, key).Result()
}

func (c *ClusterClient) ZCard(ctx context.Context, key string) *redis.IntCmd {
	return c.Session.ZCard(ctx, key)
}

func (c *ClusterClient) Incr(ctx context.Context, key string) (int64, error) {
	return c.Session.Incr(ctx, key).Result()
}

func (c *ClusterClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.Session.Exists(ctx, keys...).Result()
}

func (c *ClusterClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.Session.Keys(ctx, pattern).Result()
}
