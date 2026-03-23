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
	BaseDir            string
	LogDir             string
	StateDir           string
	ClaudeSettingsPath string
}

func New(baseDir string) App {
	dataDir := filepath.Join(baseDir, ".ccecho")
	return App{
		BaseDir:            baseDir,
		LogDir:             filepath.Join(dataDir, "logs"),
		StateDir:           dataDir,
		ClaudeSettingsPath: filepath.Join(mustHomeDir(), ".claude", "settings.json"),
	}
}

// LoadTargetURL 加载模型HOST拼接URL
func (a App) LoadTargetURL() (string, error) {
	baseURL, err := a.LoadBaseURL()
	if err != nil {
		return "", err
	}

	path := strings.TrimRight(baseURL.Path, "/") + "/v1/messages"
	return baseURL.Host + path, nil
}

// LoadBaseURL 加载模型HOST
func (a App) LoadBaseURL() (*url.URL, error) {
	settings, err := a.LoadSettings()
	if err != nil {
		return nil, err
	}

	baseURL := settings.Env["ANTHROPIC_BASE_URL"]
	if baseURL == "" {
		return nil, fmt.Errorf("ANTHROPIC_BASE_URL not found in %s", a.ClaudeSettingsPath)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse ANTHROPIC_BASE_URL: %w", err)
	}
	return parsed, nil
}

// LoadSettings 加载claude配置
func (a App) LoadSettings() (ClaudeSettings, error) {
	raw, err := os.ReadFile(a.ClaudeSettingsPath)
	if err != nil {
		return ClaudeSettings{}, fmt.Errorf("read settings: %w", err)
	}

	var settings ClaudeSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return ClaudeSettings{}, fmt.Errorf("parse settings: %w", err)
	}
	if settings.Env == nil {
		settings.Env = map[string]string{}
	}
	return settings, nil
}

// 获取用户目录
func mustHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return home
}
