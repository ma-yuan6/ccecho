// Package logstore 负责按会话目录组织代理过程中的请求与响应日志文件。
// 它管理当前会话目录、请求序号分配，以及各类日志文件的落盘。
package logstore

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const defaultQueueSize = 128

type writeTask struct {
	path string
	data []byte
}

// Store 表示单个代理会话的日志存储器
// 一个 Store 对应 logs/<session>/ 下的一组请求、响应和元信息文件
type Store struct {
	logDir      string
	mu          sync.Mutex
	counter     int
	currentName string
	currentPath string
	writes      chan writeTask
	done        chan struct{}
	closed      bool
}

// New 创建一个新的会话日志存储器，并确保日志根目录与会话目录存在
// name 通常是时间戳形式的 session 名称
func New(logDir string, name string) (*Store, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("[error] [error] create log dir: %w", err)
	}

	path := filepath.Join(logDir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("[error] [error] create session dir: %w", err)
	}

	store := &Store{
		logDir:      logDir,
		currentName: name,
		currentPath: path,
		writes:      make(chan writeTask, defaultQueueSize),
		done:        make(chan struct{}),
	}
	go store.runWriter()
	return store, nil
}

// SessionName 返回当前日志会话名称
func (s *Store) SessionName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentName
}

// NextIndex 返回下一个请求序号
// 每次代理收到一个新请求时调用一次，用于生成 requestN/responseN 文件名
func (s *Store) NextIndex() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	return s.counter
}

// CurrentPath 返回当前会话日志目录路径
func (s *Store) CurrentPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentPath
}

// WriteRequest 将第 idx 个请求体写入 request<idx>.json
func (s *Store) WriteRequest(idx int, data []byte) error {
	return s.enqueue(filepath.Join(s.currentPath, fmt.Sprintf("request%d.json", idx)), data)
}

// WriteResponseStream 将第 idx 个流式响应写入 response<idx>.stream
func (s *Store) WriteResponseStream(idx int, data []byte) error {
	return s.enqueue(filepath.Join(s.currentPath, fmt.Sprintf("response%d.stream", idx)), data)
}

// WriteResponseJSON 将第 idx 个非流式或已收敛的响应写入 response<idx>.json
func (s *Store) WriteResponseJSON(idx int, data []byte) error {
	return s.enqueue(filepath.Join(s.currentPath, fmt.Sprintf("response%d.json", idx)), data)
}

// Close 停止接收新的写入任务，并等待后台写盘队列清空
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.done
		return nil
	}
	s.closed = true
	close(s.writes)
	s.mu.Unlock()

	<-s.done
	return nil
}

// 将文件写入加入队列
func (s *Store) enqueue(path string, data []byte) error {
	task := writeTask{
		path: path,
		data: slices.Clone(data),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("[error] log store closed")
	}
	s.writes <- task
	return nil
}

// 文件写入 goroutine
func (s *Store) runWriter() {
	defer close(s.done)
	for task := range s.writes {
		if err := os.WriteFile(task.path, task.data, 0o644); err != nil {
			log.Printf("[error] write log file %s: %v", task.path, err)
		}
	}
}
