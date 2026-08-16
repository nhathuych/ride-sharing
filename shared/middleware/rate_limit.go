package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"ride-sharing/shared/logger"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const slidingWindowScript = `
local key = KEYS[1]

local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

local min_score = now - window

-- Remove requests outside the sliding window.
redis.call("ZREMRANGEBYSCORE", key, "-inf", min_score)

local count = redis.call("ZCARD", key)

if count >= limit then
	local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
	local oldest_score = tonumber(oldest[2])

	local retry_after = window - (now - oldest_score)

	return {0, count, retry_after}
end

redis.call("ZADD", key, now, member)

-- Keep the key alive slightly longer than the window.
local ttl = math.ceil(window / 1000) + 1
redis.call("EXPIRE", key, ttl)

return {1, count + 1, 0}
`

var rateLimitScript = redis.NewScript(slidingWindowScript)

type RateLimiter struct {
	Redis    *redis.Client
	Limit    int
	Window   time.Duration
	KeyFunc  func(*http.Request) string
	FailOpen bool
}

func NewRateLimiter(
	redisClient *redis.Client,
	limit int,
	window time.Duration,
	keyFunc func(*http.Request) string,
) *RateLimiter {
	return &RateLimiter{
		Redis:    redisClient,
		Limit:    limit,
		Window:   window,
		KeyFunc:  keyFunc,
		FailOpen: true,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.Redis == nil {
			next.ServeHTTP(w, r)
			return
		}

		key := rl.KeyFunc(r)

		allowed, remaining, retryAfter, err := rl.Allow(r.Context(), key)

		if err != nil {
			logger.WithTrace(r.Context()).Error("rate limiter error", zap.Error(err), zap.String("key", key))

			if rl.FailOpen {
				next.ServeHTTP(w, r)
				return
			}

			http.Error(
				w,
				http.StatusText(http.StatusServiceUnavailable),
				http.StatusServiceUnavailable,
			)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.Limit))

		remaining = max(remaining, 0)
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			retrySeconds := int64((retryAfter + time.Second - 1) / time.Second)
			retrySeconds = max(retrySeconds, 1)

			w.Header().Set("Retry-After", strconv.FormatInt(retrySeconds, 10))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)

			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) Allow(
	ctx context.Context,
	identifier string,
) (allowed bool, remaining int, retryAfter time.Duration, err error) {
	now := time.Now().UnixMilli()
	window := rl.Window.Milliseconds()

	member := randomMember()
	key := "rate-limit:" + identifier

	result, err := rateLimitScript.Run(
		ctx,
		rl.Redis,
		[]string{key},
		now,
		window,
		rl.Limit,
		member,
	).Result()

	if err != nil {
		return false, 0, 0, err
	}

	values, ok := result.([]any)
	if !ok || len(values) != 3 {
		return false, 0, 0, redis.Nil
	}

	allowedValue, ok := values[0].(int64)
	if !ok {
		return false, 0, 0, redis.Nil
	}

	count, ok := values[1].(int64)
	if !ok {
		return false, 0, 0, redis.Nil
	}

	retryAfterMs, ok := values[2].(int64)
	if !ok {
		return false, 0, 0, redis.Nil
	}

	remaining = rl.Limit - int(count)
	remaining = max(remaining, 0)

	return allowedValue == 1,
		remaining,
		time.Duration(retryAfterMs) * time.Millisecond,
		nil
}

func randomMember() string {
	buf := make([]byte, 16)

	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	return hex.EncodeToString(buf)
}

func IPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return "ip:" + host
	}

	return "ip:" + r.RemoteAddr
}

func UserKey(userID func(*http.Request) string) func(*http.Request) string {
	return func(r *http.Request) string {
		return "user:" + userID(r)
	}
}

func RouteIPKey(r *http.Request) string {
	route := requestRoute(r)

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	return "route:" + route + ":ip:" + host
}
