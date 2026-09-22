package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type SingleClient struct {
	Host    string
	Session *redis.Client
}

func CreateRedisSession(host string) (Client, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: host,
		DB:   0,
	})
	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		return nil, err
	}
	return &SingleClient{
		Host:    host,
		Session: redisClient,
	}, nil
}

func (c *SingleClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.Session.Set(ctx, key, value, expiration).Err()
}

func (c *SingleClient) Get(ctx context.Context, key string) (string, error) {
	return c.Session.Get(ctx, key).Result()
}

func (c *SingleClient) Del(ctx context.Context, key string) error {
	return c.Session.Del(ctx, key).Err()
}

func (c *SingleClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.Session.Expire(ctx, key, expiration).Result()
}

func (c *SingleClient) RPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.Session.RPush(ctx, key, values...).Result()
}

func (c *SingleClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.Session.LRange(ctx, key, start, stop).Result()
}

func (c *SingleClient) LTrim(ctx context.Context, key string, start, stop int64) (string, error) {
	return c.Session.LTrim(ctx, key, start, stop).Result()
}

func (c *SingleClient) LLen(ctx context.Context, key string) (int64, error) {
	return c.Session.LLen(ctx, key).Result()
}

func (c *SingleClient) Close() error {
	return c.Session.Close()
}

func (c *SingleClient) ZRevRangeWithScores(ctx context.Context, key string, start int64, stop int64) *redis.ZSliceCmd {
	return c.Session.ZRevRangeWithScores(ctx, key, start, stop)
}

func (c *SingleClient) ZRevRank(ctx context.Context, key string, member string) (int64, error) {
	return c.Session.ZRevRank(ctx, key, member).Result()
}

func (c *SingleClient) ZScore(ctx context.Context, key string, member string) (float64, error) {
	return c.Session.ZScore(ctx, key, member).Result()
}

func (c *SingleClient) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.Session.HSet(ctx, key, values).Result()
}

func (c *SingleClient) HDel(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.Session.HDel(ctx, key).Result()
}

func (c *SingleClient) HGet(ctx context.Context, key string, field string) (string, error) {
	return c.Session.HGet(ctx, key, field).Result()
}

func (c *SingleClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.Session.HGetAll(ctx, key).Result()
}
func (c *SingleClient) ZCard(ctx context.Context, key string) *redis.IntCmd {
	return c.Session.ZCard(ctx, key)
}

func (c *SingleClient) Incr(ctx context.Context, key string) (int64, error) {
	return c.Session.Incr(ctx, key).Result()
}

func (c *SingleClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.Session.Exists(ctx, keys...).Result()
}

func (c *SingleClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.Session.Keys(ctx, pattern).Result()
}

func CreateRedisSessionFromURL(redisURL string) (Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	redisClient := redis.NewClient(opts)

	// Use a context with timeout for the initial ping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		return nil, err
	}

	return &SingleClient{
		Host:    opts.Addr,
		Session: redisClient,
	}, nil
}
