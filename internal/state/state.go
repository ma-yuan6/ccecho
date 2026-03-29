// Package state 管理当前活跃代理会话的状态指针。
// 它只负责读写当前会话入口信息，不负责具体请求/响应日志落盘。
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Current struct {
	SessionName     string `json:"session_name"`
	SessionPath     string `json:"session_path"`
	ProxyAddr       string `json:"proxy_addr"`
	ProxyCode       string `json:"proxy_code"`
	SettingsPath    string `json:"settings_path"`
	SettingsBackup  string `json:"settings_backup"`
	OriginalBaseURL string `json:"original_base_url"`
}

// Write 将当前活跃会话状态写入 stateDir/current_session.json。
// 该文件用于让 view 等命令快速定位最近一次启动的会话。
func Write(stateDir string, current Current) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("[error] create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("[error] marshal state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "current_session.json"), raw, 0o644); err != nil {
		return fmt.Errorf("[error] write state: %w", err)
	}
	return nil
}

// Read 从 stateDir/current_session.json 读取当前活跃会话状态。
// 当状态文件不存在或内容非法时返回错误。
func Read(stateDir string) (Current, error) {
	raw, err := os.ReadFile(filepath.Join(stateDir, "current_session.json"))
	if err != nil {
		return Current{}, err
	}
	var current Current
	if err := json.Unmarshal(raw, &current); err != nil {
		return Current{}, err
	}
	return current, nil
}
