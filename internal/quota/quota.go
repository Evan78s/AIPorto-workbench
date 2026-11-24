package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"aiporto/internal/auth"
)

// Acquire 在一个 Lua 脚本内原子完成：RPM 计数、并发占位、当日 token 预扣。
// 任何一项超限都会回滚已经做的变更，避免"部分扣减"。
var acquireScript = redis.NewScript(`
local rpmKey   = KEYS[1]
local concKey  = KEYS[2]
local tokKey   = KEYS[3]
local rpmLimit = tonumber(ARGV[1])
local concLimit = tonumber(ARGV[2])
local dailyLimit = tonumber(ARGV[3])
local reqID    = ARGV[4]
local now      = tonumber(ARGV[5])
local estimate = tonumber(ARGV[6])
local midnight = tonumber(ARGV[7])

local function rollbackRPM()
  redis.call('DECR', rpmKey)
end

local rpm = redis.call('INCR', rpmKey)
if rpm == 1 then redis.call('EXPIRE', rpmKey, 60) end
if rpmLimit > 0 and rpm > rpmLimit then
  rollbackRPM()
  local ttl = redis.call('TTL', rpmKey)
  if ttl < 0 then ttl = 60 end
  return {0, 'rpm', ttl}
end

redis.call('ZREMRANGEBYSCORE', concKey, '-inf', now - 3600000)
local conc = redis.call('ZCARD', concKey)
if concLimit > 0 and conc >= concLimit then
  rollbackRPM()
  return {0, 'concurrency', 60}
end
redis.call('ZADD', concKey, now, reqID)
redis.call('PEXPIRE', concKey, 3600000)

if dailyLimit > 0 and estimate > 0 then
  local used = tonumber(redis.call('GET', tokKey) or '0')
  if used + estimate > dailyLimit then
    rollbackRPM()
    redis.call('ZREM', concKey, reqID)
    local ttl = redis.call('TTL', tokKey)
    if ttl < 0 then ttl = midnight - math.floor(now / 1000) end
    if ttl < 1 then ttl = 1 end
    return {0, 'tokens', math.floor(ttl)}
  end
  redis.call('INCRBY', tokKey, estimate)
  if redis.call('TTL', tokKey) < 0 then
    redis.call('EXPIREAT', tokKey, midnight)
  end
end
return {1, 'ok', 0}
`)

// Settle 在请求结束后释放并发占位，并把预扣量校正为真实用量。
var settleScript = redis.NewScript(`
local tokKey   = KEYS[1]
local concKey  = KEYS[2]
local reqID    = ARGV[1]
local estimate = tonumber(ARGV[2])
local actual   = tonumber(ARGV[3])
local midnight = tonumber(ARGV[4])

redis.call('ZREM', concKey, reqID)
local delta = actual - estimate
if delta ~= 0 then redis.call('INCRBY', tokKey, delta) end
if redis.call('TTL', tokKey) < 0 then redis.call('EXPIREAT', tokKey, midnight) end
local v = tonumber(redis.call('GET', tokKey) or '0')
if v < 0 then redis.call('SET', tokKey, 0, 'KEEPTTL') end
return v
`)

type Decision struct {
	Allowed    bool
	Reason     string
	RetryAfter int
}

type Limiter struct {
	rdb redis.UniversalClient
}

func New(rdb redis.UniversalClient) *Limiter {
	return &Limiter{rdb: rdb}
}

func (l *Limiter) Acquire(ctx context.Context, id auth.Identity, reqID string, estimate int) (Decision, error) {
	now := time.Now()
	midnight := nextMidnight(now)
	keys := []string{
		"gw:rpm:" + id.KeyID,
		"gw:conc:" + id.KeyID,
		fmt.Sprintf("gw:tok:%s:%s", id.KeyID, now.Format("20060102")),
	}
	args := []any{id.RpmLimit, id.ConcurrencyLimit, id.DailyTokenLimit, reqID, now.UnixMilli(), estimate, midnight}

	vals, err := acquireScript.Run(ctx, l.rdb, keys, args...).Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(vals) < 3 {
		return Decision{}, fmt.Errorf("unexpected acquire result length %d", len(vals))
	}
	ok, _ := vals[0].(int64)
	reason, _ := vals[1].(string)
	retry, _ := vals[2].(int64)
	return Decision{Allowed: ok == 1, Reason: reason, RetryAfter: int(retry)}, nil
}

// Settle 使用 context.Background()，因为客户端断开后 request context 可能已取消。
func (l *Limiter) Settle(ctx context.Context, id auth.Identity, reqID string, estimate, actual int) {
	now := time.Now()
	midnight := nextMidnight(now)
	keys := []string{
		fmt.Sprintf("gw:tok:%s:%s", id.KeyID, now.Format("20060102")),
		"gw:conc:" + id.KeyID,
	}
	args := []any{reqID, estimate, actual, midnight}
	_, _ = settleScript.Run(ctx, l.rdb, keys, args...).Result()
}

func nextMidnight(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location()).Unix()
}
