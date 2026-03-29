// Package sessionmeta  session 元数据
package sessionmeta

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"claude-proxy-go/internal/config"
)

const FileName = "meta.json"

type Meta struct {
	SessionName  string `json:"session_name"`
	SessionPath  string `json:"session_path"`
	Provider     string `json:"provider"`
	TargetName   string `json:"target_name,omitempty"`
	Target       string `json:"target"`
	ProxyAddr    string `json:"proxy_addr"`
	ProxyCode    string `json:"proxy_code,omitempty"`
	RoutePrefix  string `json:"route_prefix,omitempty"`
	LocalBaseURL string `json:"local_base_url"`
	CreatedAt    string `json:"created_at"`
}

func ValidateProvider(provider string) error {
	switch provider {
	case config.ProviderClaude, config.ProviderCodex:
		return nil
	default:
		return fmt.Errorf("[error] unsupported provider: %q", provider)
	}
}

func Read(dir string) (Meta, error) {
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Meta{}, err
	}
	if err := ValidateProvider(meta.Provider); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func Write(dir string, meta Meta) error {
	if err := ValidateProvider(meta.Provider); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), raw, 0o644)
}
