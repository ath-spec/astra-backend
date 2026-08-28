package connectors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis connector. Live and ready to use; not yet wired into cmd/api because
// Astra runs a single API instance and the Postgres fingerprint cache covers the
// narrative use case. Wire it in when there is cross-replica state to share:
// rate limiting / login throttling, a shared response cache, or Bedrock Agent
// session state.
//
//	cache, err := connectors.CreateRedisFromURL(ctx, cfg.RedisURL) // REDIS_URL
//	if err != nil { ... }                                          // or fall back:
//	cache := connectors.NoopCache{}                                // no-op, always safe
//
// On AWS point REDIS_URL at ElastiCache with the rediss:// scheme so TLS is on.
// Ported from z-backend server/common/redis + server/common/connectors/redis.go,
// with the same bounded-retry boot policy as CreatePostgresPool.

// Redis pool tuning — conservative production defaults, applied only when the URL
// does not already specify them.
const (
	redisPoolSize        = 20
	redisMinIdleConns    = 4
	redisConnMaxLifetime = 30 * time.Minute
	redisConnMaxIdleTime = 5 * time.Minute
	redisDialTimeout     = 5 * time.Second
	redisReadTimeout     = 3 * time.Second
	redisWriteTimeout    = 3 * time.Second
	redisPingTimeout     = 5 * time.Second
)

// ErrCacheMiss is returned by Cache.Get when the key does not exist. Callers
// should branch on this rather than treating every error as fatal.
var ErrCacheMiss = errors.New("connectors: cache miss")

// Cache is the narrow slice of Redis the service needs. Keep it small — widen
// only when a caller genuinely needs another primitive, so NoopCache and any
// test double stay trivial to maintain.
type Cache interface {
	// Get returns ErrCacheMiss (not a nil string) when the key is absent.
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// Incr / IncrBy back rate limiters and counters. A fresh key starts at 0.
	Incr(ctx context.Context, key string) (int64, error)
	IncrBy(ctx context.Context, key string, n int64) (int64, error)

	// List ops — a lightweight FIFO before a real broker is justified.
	RPush(ctx context.Context, key string, values ...any) (int64, error)
	LPop(ctx context.Context, key string) (string, error)
	LLen(ctx context.Context, key string) (int64, error)

	Ping(ctx context.Context) error
	Close() error
}

type redisCache struct{ rdb *redis.Client }

// CreateRedisFromURL parses a redis:// or rediss:// URL, applies the production
// pool defaults, and returns a Ping-verified client — retrying a bounded number
// of times so a cache that is still coming up at boot does not crash-loop the
// process (same policy as CreatePostgresPool).
func CreateRedisFromURL(ctx context.Context, url string) (Cache, error) {
	if url == "" {
		return nil, fmt.Errorf("redis url is empty")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("unable to parse redis url: %w", err)
	}
	applyRedisDefaults(opts)

	var lastErr error
	for attempt := 1; attempt <= connectRetries; attempt++ {
		rdb := redis.NewClient(opts)

		pingCtx, cancel := context.WithTimeout(ctx, redisPingTimeout)
		_, perr := rdb.Ping(pingCtx).Result()
		cancel()

		if perr == nil {
			slog.Info("connected to redis",
				"addr", opts.Addr,
				"tls", opts.TLSConfig != nil,
				"pool_size", opts.PoolSize,
			)
			return &redisCache{rdb: rdb}, nil
		}
		_ = rdb.Close()
		lastErr = perr

		if ctx.Err() != nil {
			return nil, fmt.Errorf("unable to connect to redis: %w", ctx.Err())
		}
		if attempt < connectRetries {
			slog.Warn("redis not ready, retrying",
				"addr", sanitizeRedisURL(url),
				"attempt", attempt, "of", connectRetries, "error", perr)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("unable to connect to redis: %w", ctx.Err())
			case <-time.After(connectRetryDelay):
			}
		}
	}

	return nil, fmt.Errorf("unable to connect to redis after %d attempts: %w", connectRetries, lastErr)
}

func applyRedisDefaults(o *redis.Options) {
	if o.PoolSize == 0 {
		o.PoolSize = redisPoolSize
	}
	if o.MinIdleConns == 0 {
		o.MinIdleConns = redisMinIdleConns
	}
	if o.ConnMaxLifetime == 0 {
		o.ConnMaxLifetime = redisConnMaxLifetime
	}
	if o.ConnMaxIdleTime == 0 {
		o.ConnMaxIdleTime = redisConnMaxIdleTime
	}
	if o.DialTimeout == 0 {
		o.DialTimeout = redisDialTimeout
	}
	if o.ReadTimeout == 0 {
		o.ReadTimeout = redisReadTimeout
	}
	if o.WriteTimeout == 0 {
		o.WriteTimeout = redisWriteTimeout
	}
}

// sanitizeRedisURL hides the password for log lines.
func sanitizeRedisURL(url string) string {
	at := strings.LastIndex(url, "@")
	if at == -1 {
		return url
	}
	scheme := strings.Index(url, "://")
	if scheme == -1 {
		return "****" + url[at:]
	}
	return url[:scheme+3] + "****" + url[at:]
}

func (c *redisCache) Get(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	return v, err
}

func (c *redisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *redisCache) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *redisCache) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.rdb.Exists(ctx, keys...).Result()
}

func (c *redisCache) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return c.rdb.Expire(ctx, key, ttl).Result()
}

func (c *redisCache) Incr(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *redisCache) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return c.rdb.IncrBy(ctx, key, n).Result()
}

func (c *redisCache) RPush(ctx context.Context, key string, values ...any) (int64, error) {
	return c.rdb.RPush(ctx, key, values...).Result()
}

func (c *redisCache) LPop(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.LPop(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	return v, err
}

func (c *redisCache) LLen(ctx context.Context, key string) (int64, error) {
	return c.rdb.LLen(ctx, key).Result()
}

func (c *redisCache) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }
func (c *redisCache) Close() error                   { return c.rdb.Close() }

// NoopCache is a Cache that stores nothing: every read is a miss, every write
// succeeds silently. It lets call sites be written against Cache now and wired to
// a real Redis later without nil checks. Not a correctness substitute for Redis
// where cross-replica consistency is the point (e.g. rate limiting).
type NoopCache struct{}

func (NoopCache) Get(context.Context, string) (string, error)           { return "", ErrCacheMiss }
func (NoopCache) Set(context.Context, string, any, time.Duration) error { return nil }
func (NoopCache) Del(context.Context, ...string) error                  { return nil }
func (NoopCache) Exists(context.Context, ...string) (int64, error)      { return 0, nil }
func (NoopCache) Expire(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}
func (NoopCache) Incr(context.Context, string) (int64, error)          { return 0, nil }
func (NoopCache) IncrBy(context.Context, string, int64) (int64, error) { return 0, nil }
func (NoopCache) RPush(context.Context, string, ...any) (int64, error) { return 0, nil }
func (NoopCache) LPop(context.Context, string) (string, error)         { return "", ErrCacheMiss }
func (NoopCache) LLen(context.Context, string) (int64, error)          { return 0, nil }
func (NoopCache) Ping(context.Context) error                           { return nil }
func (NoopCache) Close() error                                         { return nil }

// compile-time checks
var (
	_ Cache = (*redisCache)(nil)
	_ Cache = NoopCache{}
)
