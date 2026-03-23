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
	SettingsPath    string `json:"settings_path"`
	SettingsBackup  string `json:"settings_backup"`
	OriginalBaseURL string `json:"original_base_url"`
}

func Write(stateDir string, current Current) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "current_session.json"), raw, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

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
