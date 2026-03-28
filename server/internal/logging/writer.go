package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingWriter 按天轮转的文件写入器。
// 每次写入时检测日期变化，自动切换到新文件。
type RotatingWriter struct {
	mu      sync.Mutex
	dir     string
	subdir  string // 子目录: "system", "audit", "agent/{host_id}"
	curDate string
	f       *os.File
	offset  int64
}

// NewRotatingWriter 创建一个按天轮转写入器。
// dir 是日志根目录，subdir 是子目录（如 "system"、"audit"、"agent/srv-71"）。
func NewRotatingWriter(dir, subdir string) (*RotatingWriter, error) {
	w := &RotatingWriter{dir: dir, subdir: subdir}
	if err := w.openForDate(time.Now().UTC().Format("2006-01-02")); err != nil {
		return nil, err
	}
	return w, nil
}

// Write 写入一行（line 不含换行符，Write 会自动追加 \n）。
// 返回写入前的文件偏移量和行字节长度（含换行）。
func (w *RotatingWriter) Write(line []byte) (offset int64, length int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if today != w.curDate {
		if err = w.openForDate(today); err != nil {
			return 0, 0, err
		}
	}

	// 加换行符
	data := make([]byte, len(line)+1)
	copy(data, line)
	data[len(line)] = '\n'

	offset = w.offset
	n, err := w.f.Write(data)
	if err != nil {
		return 0, 0, err
	}
	w.offset += int64(n)
	return offset, n, nil
}

// FilePath 返回当前日期日志文件的相对路径（相对于日志根目录）。
func (w *RotatingWriter) FilePath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return filepath.Join(w.subdir, w.curDate+".log")
}

// Close 关闭当前文件。
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		return w.f.Close()
	}
	return nil
}

func (w *RotatingWriter) openForDate(date string) error {
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}

	dir := filepath.Join(w.dir, w.subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("logging: mkdir %s: %w", dir, err)
	}

	fpath := filepath.Join(dir, date+".log")
	f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("logging: open %s: %w", fpath, err)
	}

	// P3 fix: 用 f.Stat() 获取偏移量，避免 Seek
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("logging: stat %s: %w", fpath, err)
	}

	w.f = f
	w.curDate = date
	w.offset = fi.Size()
	return nil
}
