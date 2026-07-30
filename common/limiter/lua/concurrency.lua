-- 两级并发限制（账户级 + 令牌级），单次往返内原子完成。
--
-- 为什么不用 INCR/DECR 计数器：进程被 SIGKILL / OOM 杀掉时 release 跑不到，
-- 计数只增不减，令牌被永久锁死，只能人工清 key。ZSET 以「获取时刻」为 score，
-- 每次 acquire 前先按 score 清掉超过 TTL 的僵尸槽位，泄漏能自愈。
--
-- 为什么两级合在一个脚本：分成两次调用需要 2 次 RTT，且令牌级失败时要靠调用方
-- 回滚账户级槽位 —— 那个回滚不是原子的（回滚前的窗口里别人会看到偏高的账户计数）。
-- 合在一起后无论配几级都只有 1 次 RTT，回滚也在同一次 EVAL 内完成。
--
-- KEYS[1] = 账户级槽位集合 key（传空字符串表示该级不限制，跳过）
-- KEYS[2] = 令牌级槽位集合 key（传空字符串表示该级不限制，跳过）
-- ARGV[1] = 账户级上限
-- ARGV[2] = 令牌级上限
-- ARGV[3] = member（本次请求的唯一标识，用 requestId）
-- ARGV[4] = ttl（槽位最长存活秒数，超过即视为泄漏并回收）
--
-- 返回 0 = 通过；1 = 账户级超限；2 = 令牌级超限
--
-- Redis 版本要求：>= 3.2。脚本里调用了 TIME（非确定性命令），Redis 3.2 起脚本
-- 默认走 effect replication，可以安全调用；更早的版本会直接报错拒绝执行。
-- 用服务器时间而不是客户端传入时间戳，是为了避免多实例时钟漂移导致槽位被
-- 提前/延后回收 —— 那会让并发上限时紧时松。
local user_key   = KEYS[1]
local token_key  = KEYS[2]
local user_limit  = tonumber(ARGV[1])
local token_limit = tonumber(ARGV[2])
local member = ARGV[3]
local ttl    = tonumber(ARGV[4])

-- 用 Redis 服务器时间，避免多实例时钟漂移导致槽位被提前/延后回收
local now = tonumber(redis.call('TIME')[1])
local cutoff = now - ttl

-- 占用一个槽位；成功返回 true，已达上限返回 false
local function acquire(key, limit)
    redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)  -- 回收僵尸槽位
    if redis.call('ZCARD', key) >= limit then
        -- 已达上限也要续期，否则一个持续被打满的 key 可能在 ttl 后失去过期保护
        redis.call('EXPIRE', key, ttl)
        return false
    end
    redis.call('ZADD', key, now, member)
    -- 必须设过期时间：否则活跃 key 会永久驻留 Redis
    redis.call('EXPIRE', key, ttl)
    return true
end

-- 先判账户级。超限就直接返回，完全不碰令牌级的 key
-- （所以「两级同时超限」不存在，429 只会报一个原因）
local user_acquired = false
if user_key ~= '' and user_limit > 0 then
    if not acquire(user_key, user_limit) then
        return 1
    end
    user_acquired = true
end

if token_key ~= '' and token_limit > 0 then
    if not acquire(token_key, token_limit) then
        -- 令牌级失败 → 回滚刚占的账户级槽位。在同一次 EVAL 内完成，
        -- 外部观察不到「账户级已占用但请求已被拒」的中间状态。
        if user_acquired then
            redis.call('ZREM', user_key, member)
        end
        return 2
    end
end

return 0
