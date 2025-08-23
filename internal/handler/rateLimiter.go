package handler

import (
	"sync"
	"sync/atomic"
	"time"
)

// RateLimitHandler 创建一个高性能限流 Handler
// connRate/connBurst 每个连接的速率与突发容量
// globalRate/globalBurst 全局速率与突发容量
// limit 触发限流时回调
var RateLimitHandler = func(connRate, connBurst, globalRate, globalBurst int64, limit Handler) Handler {
	var connBuckets sync.Map
	global := NewAtomicBucket(globalRate, globalBurst)

	// 定期清理空闲连接桶
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now().UnixNano()
			connBuckets.Range(func(key, value any) bool {
				tb := value.(*AtomicBucket)
				if atomic.LoadInt64(&tb.lastAccess) < now-10*int64(time.Minute) {
					connBuckets.Delete(key)
				}
				return true
			})
		}
	}()

	return func(ctx Context, next func()) {
		connID := ctx.Conn().ID()

		val, _ := connBuckets.LoadOrStore(connID, NewAtomicBucket(connRate, connBurst))
		tb := val.(*AtomicBucket)

		if !tb.Allow() || !global.Allow() {
			if limit != nil {
				limit(ctx, next)
			}
			return
		}

		next()
	}
}

// AtomicBucket 使用原子操作实现的令牌桶
type AtomicBucket struct {
	capacity   int64 // 最大令牌数
	tokens     int64 // 当前令牌数
	refillRate int64 // 每秒补充令牌数
	lastRefill int64 // 上次补充时间（纳秒）
	lastAccess int64 // 上次访问时间（纳秒）
}

// NewAtomicBucket 创建原子令牌桶
func NewAtomicBucket(rate, burst int64) *AtomicBucket {
	now := time.Now().UnixNano()
	return &AtomicBucket{
		capacity:   burst,
		tokens:     burst,
		refillRate: rate,
		lastRefill: now,
		lastAccess: now,
	}
}

// Allow 检查是否允许通过
func (b *AtomicBucket) Allow() bool {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&b.lastRefill)
	elapsed := now - last

	// 计算补充的令牌
	if elapsed > 0 {
		newTokens := elapsed * b.refillRate / 1e9
		if newTokens > 0 {
			for {
				old := atomic.LoadInt64(&b.tokens)
				newVal := old + newTokens
				if newVal > b.capacity {
					newVal = b.capacity
				}
				if atomic.CompareAndSwapInt64(&b.tokens, old, newVal) {
					break
				}
			}
			atomic.StoreInt64(&b.lastRefill, now)
		}
	}

	// 尝试消耗一个令牌
	for {
		old := atomic.LoadInt64(&b.tokens)
		if old <= 0 {
			atomic.StoreInt64(&b.lastAccess, now)
			return false
		}
		if atomic.CompareAndSwapInt64(&b.tokens, old, old-1) {
			atomic.StoreInt64(&b.lastAccess, now)
			return true
		}
	}
}
