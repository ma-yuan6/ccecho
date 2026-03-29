package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type ClaudeSettings struct {
	Env map[string]string `json:"env"`
}

type App struct {
	BaseDir  string // 命令目录
	LogDir   string // 截取数据保存目录
	StateDir string // 数据及配置
	HomeDir  string
}

func New(baseDir string) App {
	dataDir := filepath.Join(baseDir, ".ccecho")
	return App{
		BaseDir:  baseDir,
		LogDir:   filepath.Join(dataDir, "logs"),
		StateDir: dataDir,
		HomeDir:  mustHomeDir(),
	}
}

// LoadTargetURL 加载被调用AI的HOST拼接URL
func (a App) LoadTargetURL() (string, error) {
	baseURL, err := a.LoadBaseURL()
	if err != nil {
		return "", err
	}

	path := strings.TrimRight(baseURL.Path, "/") + "/v1/messages"
	return baseURL.Host + path, nil
}

// LoadBaseURL 加载AI模型API HOST
func (a App) LoadBaseURL() (*url.URL, error) {
	settings, err := a.LoadSettings()
	if err != nil {
		return nil, err
	}

	baseURL := settings.Env["ANTHROPIC_BASE_URL"]
	if baseURL == "" {
		return nil, fmt.Errorf("[error] ANTHROPIC_BASE_URL not found in %s", a.ClaudeSettingsPath())
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("[error] parse ANTHROPIC_BASE_URL: %w", err)
	}
	return parsed, nil
}

// LoadSettings 加载claude配置
func (a App) LoadSettings() (ClaudeSettings, error) {
	raw, err := os.ReadFile(a.ClaudeSettingsPath())
	if err != nil {
		return ClaudeSettings{}, fmt.Errorf("[error] read settings: %w", err)
	}

	var settings ClaudeSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return ClaudeSettings{}, fmt.Errorf("[error] parse settings: %w", err)
	}
	if settings.Env == nil {
		settings.Env = map[string]string{}
	}
	return settings, nil
}

// ClaudeSettingsPath Claude配置文件位置
func (a App) ClaudeSettingsPath() string {
	return filepath.Join(a.HomeDir, ".claude", "settings.json")
}

// 获取用户目录
func mustHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return home
}
