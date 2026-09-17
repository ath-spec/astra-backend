package redis

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type ClusterClientTest struct {
}

// Incr implements Client.
func (c *ClusterClientTest) Incr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func CreateRedisClusterSessionTest() Client {
	return &ClusterClientTest{}
}

func (c *ClusterClientTest) ZRevRank(ctx context.Context, key string, member string) (int64, error) {
	return 0, nil
}

func (c *ClusterClientTest) ZScore(ctx context.Context, key string, member string) (float64, error) {
	return 0, nil
}

func (c *ClusterClientTest) ZRevRangeWithScores(ctx context.Context, key string, start int64, stop int64) *redis.ZSliceCmd {
	return &redis.ZSliceCmd{}
}

func (c *ClusterClientTest) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return nil
}

func (c *ClusterClientTest) Get(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (c *ClusterClientTest) Del(ctx context.Context, key string) error {
	return nil
}

func (c *ClusterClientTest) Close() error {
	return nil
}

func (c *ClusterClientTest) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return 0, nil
}

func (c *ClusterClientTest) HDel(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return 0, nil
}

func (c *ClusterClientTest) HGet(ctx context.Context, key string, field string) (string, error) {
	return "", nil
}
func (c *ClusterClientTest) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return nil, nil
}
func (c *ClusterClientTest) ZCard(ctx context.Context, key string) *redis.IntCmd {
	return nil
}

func (c *ClusterClientTest) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return true, nil
}

func (c *ClusterClientTest) RPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return 0, nil
}

func (c *ClusterClientTest) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return []string{}, nil
}

func (c *ClusterClientTest) LTrim(ctx context.Context, key string, start, stop int64) (string, error) {
	return "OK", nil
}

func (c *ClusterClientTest) LLen(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func (c *ClusterClientTest) Exists(ctx context.Context, keys ...string) (int64, error) {
	return 0, nil
}

func (c *ClusterClientTest) Keys(ctx context.Context, pattern string) ([]string, error) {
	return []string{}, nil
}
