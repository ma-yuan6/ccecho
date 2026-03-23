package logstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store 日志存储结构
type Store struct {
	logDir      string
	mu          sync.Mutex
	counter     int
	currentName string
	currentPath string
}

func New(logDir string, name string) (*Store, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("[error] create log dir: %w", err)
	}

	path := filepath.Join(logDir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("[error] create session dir: %w", err)
	}

	return &Store{
		logDir:      logDir,
		currentName: name,
		currentPath: path,
	}, nil
}

func (s *Store) SessionName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentName
}
func (s *Store) NextIndex() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	return s.counter
}

// CurrentPath 获取当前会话日志目录
func (s *Store) CurrentPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentPath
}

// WriteRequest 写入请求数据
func (s *Store) WriteRequest(idx int, data []byte) error {
	return os.WriteFile(filepath.Join(s.currentPath, fmt.Sprintf("request%d.json", idx)), data, 0o644)
}

// WriteResponseStream 写入响应数据
func (s *Store) WriteResponseStream(idx int, data []byte) error {
	return os.WriteFile(filepath.Join(s.currentPath, fmt.Sprintf("response%d.stream", idx)), data, 0o644)
}

// WriteResponseJSON 写入 JSON 响应数据
func (s *Store) WriteResponseJSON(idx int, data []byte) error {
	return os.WriteFile(filepath.Join(s.currentPath, fmt.Sprintf("response%d.json", idx)), data, 0o644)
}
