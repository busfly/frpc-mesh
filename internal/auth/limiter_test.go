package auth

import (
	"testing"
	"time"
)

func TestLimiterTieredLockout(t *testing.T) {
	l := NewLimiter()
	base := time.Now()
	key := "1.2.3.4|admin"

	for i := 1; i <= 4; i++ {
		l.Fail(key, base)
		if _, locked := l.LockedFor(key, base); locked {
			t.Fatalf("%d 次失败不应锁定", i)
		}
	}
	l.Fail(key, base) // 第 5 次 → 锁 3 分钟
	if left, locked := l.LockedFor(key, base.Add(time.Second)); !locked || left > 3*time.Minute || left < 2*time.Minute {
		t.Fatalf("5 次失败应锁 3 分钟, got locked=%v left=%v", locked, left)
	}

	// 锁定窗口过后继续错到第 10 次 → 锁 10 分钟
	t10 := base.Add(3*time.Minute + time.Hour)
	for i := 6; i <= 10; i++ {
		l.Fail(key, t10)
	}
	if left, locked := l.LockedFor(key, t10); !locked || left > 10*time.Minute || left < 9*time.Minute {
		t.Fatalf("10 次失败应锁 10 分钟, got locked=%v left=%v", locked, left)
	}

	// 第 15 次及以后 → 锁 1 小时
	t15 := t10.Add(2 * time.Hour)
	for i := 11; i <= 15; i++ {
		l.Fail(key, t15)
	}
	l.Fail(key, t15)
	if left, locked := l.LockedFor(key, t15); !locked || left > time.Hour || left < 59*time.Minute {
		t.Fatalf("15+ 次失败应锁 1 小时, got locked=%v left=%v", locked, left)
	}

	// 成功登录清零
	l.Success(key)
	if _, locked := l.LockedFor(key, t15); locked {
		t.Fatal("成功登录后应解除锁定计数")
	}
	for i := 1; i <= 4; i++ {
		l.Fail(key, t15)
	}
	if _, locked := l.LockedFor(key, t15); locked {
		t.Fatal("清零后 4 次失败不应锁定")
	}
}

func TestLimiterKeysIndependent(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < 5; i++ {
		l.Fail("1.1.1.1|admin", now)
	}
	if _, locked := l.LockedFor("2.2.2.2|admin", now); locked {
		t.Fatal("不同 IP 不应共享锁定")
	}
	if _, locked := l.LockedFor("1.1.1.1|other", now); locked {
		t.Fatal("不同用户名不应共享锁定")
	}
}
