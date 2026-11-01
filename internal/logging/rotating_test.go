package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")
	w := NewRotatingWriter(dir, "frpc-mesh.log", 1<<20, 3)
	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatalf("首次写入应成功: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "frpc-mesh.log"))
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if string(got) != "hello\n" {
		t.Errorf("内容 = %q, want %q", got, "hello\n")
	}
}

func TestRollsOverAtLimit(t *testing.T) {
	dir := t.TempDir()
	// 每条 20 字节，上限 50 字节 → 第 3 条触发滚动
	w := NewRotatingWriter(dir, "app.log", 50, 3)
	for i := 1; i <= 3; i++ {
		if _, err := fmt.Fprintf(w, "line-%02d-aaaaaaaaaaaa\n", i); err != nil {
			t.Fatalf("写入第 %d 条失败: %v", i, err)
		}
	}
	cur, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatalf("当前日志文件应存在: %v", err)
	}
	if !strings.Contains(string(cur), "line-03") {
		t.Errorf("当前文件应含最新一条, got %q", cur)
	}
	if strings.Contains(string(cur), "line-01") {
		t.Errorf("当前文件不该还含最旧一条: %q", cur)
	}
	old, err := os.ReadFile(filepath.Join(dir, "app.log.1"))
	if err != nil {
		t.Fatalf("滚动文件 app.log.1 应存在: %v", err)
	}
	if !strings.Contains(string(old), "line-01") {
		t.Errorf("app.log.1 应含被滚走的历史, got %q", old)
	}
}

func TestDropsOldestBeyondMaxFiles(t *testing.T) {
	dir := t.TempDir()
	w := NewRotatingWriter(dir, "app.log", 20, 2) // 单文件 20 字节，最多 2 份备份
	for i := 1; i <= 8; i++ {
		if _, err := fmt.Fprintf(w, "%02d-aaaaaaaaaaaaaa\n", i); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}
	for _, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s 应存在: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "app.log.3")); !os.IsNotExist(err) {
		t.Errorf("超出 maxFiles 的 app.log.3 应被删除, err=%v", err)
	}
}

func TestWriteFailsWhenDirUnusable(t *testing.T) {
	// 把"目录"指向一个普通文件 → MkdirAll 必然失败，且不依赖 root 权限绕过
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := NewRotatingWriter(filepath.Join(blocker, "logs"), "app.log", 1<<20, 3)
	if _, err := w.Write([]byte("boom")); err == nil {
		t.Fatal("目录不可用时写入应返回错误，让调用方降级")
	}
}
