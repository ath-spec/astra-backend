package connectors

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSanitizeRedisURL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"with creds", "rediss://default:s3cr3t@my-host:6379/0", "rediss://****@my-host:6379/0"},
		{"no creds", "redis://my-host:6379", "redis://my-host:6379"},
		{"no scheme with creds", "default:pw@host:6379", "****@host:6379"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeRedisURL(c.in); got != c.want {
				t.Fatalf("sanitizeRedisURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestApplyRedisDefaults(t *testing.T) {
	t.Run("fills zero values", func(t *testing.T) {
		o := &redis.Options{}
		applyRedisDefaults(o)
		if o.PoolSize != redisPoolSize || o.MinIdleConns != redisMinIdleConns {
			t.Fatalf("pool sizing not applied: %+v", o)
		}
		if o.DialTimeout != redisDialTimeout || o.ReadTimeout != redisReadTimeout || o.WriteTimeout != redisWriteTimeout {
			t.Fatalf("timeouts not applied: %+v", o)
		}
	})
	t.Run("respects caller-set values", func(t *testing.T) {
		o := &redis.Options{PoolSize: 3, DialTimeout: time.Second}
		applyRedisDefaults(o)
		if o.PoolSize != 3 || o.DialTimeout != time.Second {
			t.Fatalf("overrode caller values: %+v", o)
		}
	})
}

func TestCreateRedisFromURLValidation(t *testing.T) {
	if _, err := CreateRedisFromURL(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty url")
	}
	if _, err := CreateRedisFromURL(context.Background(), "not-a-url://%%%"); err == nil {
		t.Fatal("expected parse error for malformed url")
	}
}

func TestNoopCache(t *testing.T) {
	ctx := context.Background()
	var c Cache = NoopCache{}

	if _, err := c.Get(ctx, "k"); err != ErrCacheMiss {
		t.Fatalf("Get on NoopCache = %v, want ErrCacheMiss", err)
	}
	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("Set on NoopCache = %v, want nil", err)
	}
	if n, err := c.Incr(ctx, "k"); err != nil || n != 0 {
		t.Fatalf("Incr on NoopCache = (%d, %v), want (0, nil)", n, err)
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping on NoopCache = %v, want nil", err)
	}
}
