package auth

import (
	"sync"
	"time"
)

type lockTier struct {
	threshold int
	duration  time.Duration
}

// Limiter 按 key（IP+用户名）统计连续登录失败并逐级锁定：
// 连续 5 次失败锁 3 分钟，10 次锁 10 分钟，15 次及以后锁 1 小时；成功登录即清零。
type Limiter struct {
	mu     sync.Mutex
	tiers  []lockTier
	records map[string]*loginRecord
	lastGC time.Time
}

type loginRecord struct {
	fails     int
	until     time.Time
	expiresAt time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{
		tiers: []lockTier{
			{5, 3 * time.Minute},
			{10, 10 * time.Minute},
			{15, time.Hour},
		},
		records: map[string]*loginRecord{},
	}
}

// LockedFor 返回该 key 当前是否被锁定及剩余时长。
func (l *Limiter) LockedFor(key string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.records[key]
	if !ok || now.After(rec.until) {
		return 0, false
	}
	return rec.until.Sub(now).Round(time.Second), true
}

func (l *Limiter) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked(now)
	rec := l.records[key]
	if rec == nil {
		rec = &loginRecord{}
		l.records[key] = rec
	}
	rec.fails++
	rec.expiresAt = now.Add(time.Hour).Add(time.Hour)
	for _, t := range l.tiers {
		if rec.fails >= t.threshold {
			rec.until = now.Add(t.duration)
		}
	}
}

func (l *Limiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, key)
}

func (l *Limiter) gcLocked(now time.Time) {
	if now.Sub(l.lastGC) < 10*time.Minute {
		return
	}
	for k, rec := range l.records {
		if now.After(rec.expiresAt) && now.After(rec.until) {
			delete(l.records, k)
		}
	}
	l.lastGC = now
}
