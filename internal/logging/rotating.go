// Package logging 提供带大小滚动的日志写入器，供面板运行日志落盘使用。
package logging

import (
	"fmt"
	"os"
	"path/filepath"
)

// RotatingWriter 写入 <dir>/<name>：单文件超过 maxSize 时滚动为 .1 … .maxFiles，
// 超出份数的最旧文件被删除。写入不经缓冲，进程退出无需显式关闭。
type RotatingWriter struct {
	dir      string
	name     string
	maxSize  int64
	maxFiles int
	f        *os.File
	size     int64
}

func NewRotatingWriter(dir, name string, maxSize int64, maxFiles int) *RotatingWriter {
	return &RotatingWriter{dir: dir, name: name, maxSize: maxSize, maxFiles: maxFiles}
}

func (w *RotatingWriter) path() string { return filepath.Join(w.dir, w.name) }

// Write 实现 io.Writer。目录/文件在首次写入时创建，失败向上返回错误由调用方降级。
func (w *RotatingWriter) Write(p []byte) (int, error) {
	if err := w.ensureOpen(); err != nil {
		return 0, err
	}
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
		if err := w.ensureOpen(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) ensureOpen() error {
	if w.f != nil {
		return nil
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录 %s 失败: %w", w.dir, err)
	}
	f, err := os.OpenFile(w.path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("打开日志文件 %s 失败: %w", w.path(), err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.f, w.size = f, st.Size()
	return nil
}

// rotate 关闭当前文件后，把 .(maxFiles-1)→.max、…、.1→.2、本体→.1 依次后移。
func (w *RotatingWriter) rotate() error {
	if w.f != nil {
		err := w.f.Close()
		w.f, w.size = nil, 0
		if err != nil {
			return fmt.Errorf("关闭日志文件失败: %w", err)
		}
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", w.path(), w.maxFiles))
	for i := w.maxFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path(), i)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, fmt.Sprintf("%s.%d", w.path(), i+1)); err != nil {
			return fmt.Errorf("滚动日志失败: %w", err)
		}
	}
	if err := os.Rename(w.path(), w.path()+".1"); err != nil {
		return fmt.Errorf("滚动日志失败: %w", err)
	}
	return nil
}
