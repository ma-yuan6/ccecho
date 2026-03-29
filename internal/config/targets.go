package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	ProviderClaude = "claude"
	ProviderCodex  = "codex"
)

// Target 表示代理最终要转发到的上游目标。
// 对 Claude 来说 Name 固定为 "claude"；
// 对 Codex 来说 Name 是 ~/.codex/config.toml 中选中的 provider 名称
type Target struct {
	Provider string
	Name     string
	BaseURL  *url.URL
}

// CodexConfig 表示 ~/.codex/config.toml 中与 provider 选择相关的顶层配置
// 典型示例:
//
//	model_provider = "niucode"
//
//	[model_providers.niucode]
//	name = "Niucode"
//	base_url = "https://api.niucode.example/v1"
//	wire_api = "responses"
//	requires_openai_auth = false
//
//	[model_providers.openai]
//	name = "OpenAI"
//	base_url = "https://api.openai.com/v1"
//	wire_api = "responses"
//	requires_openai_auth = true
//
// 其中:
// - ModelProvider  是当前默认选中的 provider 名称，大概率是 model_provider 的值
// - ModelProviders 保存各个 [model_providers.<name>] 小节的具体配置
type CodexConfig struct {
	ModelProvider  string
	ModelProviders map[string]CodexProviderConfig
}

// CodexProviderConfig 表示 ~/.codex/config.toml 中单个 [model_providers.<name>] 小节
// 例如 [model_providers.niucode] 对应的就是一个 CodexProviderConfig。
type CodexProviderConfig struct {
	Name               string
	BaseURL            string
	WireAPI            string
	RequiresOpenAIAuth bool
}

// LoadTarget 根据 provider 类型加载对应的上游目标配置
// provider 为空时默认按 Claude 处理；codexProvider 仅在 provider=codex 时生效
func (a App) LoadTarget(provider string, codexProvider string) (Target, error) {
	switch provider {
	case "", ProviderClaude:
		return a.LoadClaudeTarget()
	case ProviderCodex:
		return a.LoadCodexTarget(codexProvider)
	default:
		return Target{}, fmt.Errorf("[error] unsupported provider: %s", provider)
	}
}

// LoadClaudeTarget 从 Claude 配置中读取当前上游 base URL，并构造 Claude 目标
func (a App) LoadClaudeTarget() (Target, error) {
	baseURL, err := a.LoadBaseURL()
	if err != nil {
		return Target{}, err
	}
	return Target{
		Provider: ProviderClaude,
		Name:     ProviderClaude,
		BaseURL:  baseURL,
	}, nil
}

// LoadCodexTarget 从 ~/.codex/config.toml 中解析当前 Codex provider，并构造目标
// 当 providerOverride 非空时优先使用它；否则回退到顶层的 model_provider
func (a App) LoadCodexTarget(providerOverride string) (Target, error) {
	cfg, err := a.LoadCodexConfig()
	if err != nil {
		return Target{}, err
	}

	providerName := providerOverride
	if providerName == "" {
		providerName = cfg.ModelProvider
	}
	if providerName == "" {
		return Target{}, fmt.Errorf("[error] model_provider not found in %s", a.CodexConfigPath())
	}

	provider, ok := cfg.ModelProviders[providerName]
	if !ok {
		return Target{}, fmt.Errorf("[error] model provider %q not found in %s", providerName, a.CodexConfigPath())
	}
	if provider.BaseURL == "" {
		return Target{}, fmt.Errorf("[error] base_url not found for provider %q in %s", providerName, a.CodexConfigPath())
	}

	baseURL, err := url.Parse(provider.BaseURL)
	if err != nil {
		return Target{}, fmt.Errorf("[error] parse base_url for provider %q: %w", providerName, err)
	}
	return Target{
		Provider: ProviderCodex,
		Name:     providerName,
		BaseURL:  baseURL,
	}, nil
}

// LoadCodexConfig 读取并解析 ~/.codex/config.toml 中 ccecho 关心的字段
func (a App) LoadCodexConfig() (CodexConfig, error) {
	raw, err := os.ReadFile(a.CodexConfigPath())
	if err != nil {
		return CodexConfig{}, fmt.Errorf("[error] read codex config: %w", err)
	}
	return parseCodexConfig(raw), nil
}

// DefaultCodexProviderName 返回 ~/.codex/config.toml 中的默认 provider 名称
// 当配置缺失或读取失败时返回空字符串。
func (a App) DefaultCodexProviderName() string {
	cfg, err := a.LoadCodexConfig()
	if err != nil {
		return ""
	}
	return cfg.ModelProvider
}

// CodexConfigPath 返回当前用户 Codex 配置文件路径
func (a App) CodexConfigPath() string {
	return filepath.Join(a.HomeDir, ".codex", "config.toml")
}

// parseCodexConfig 从原始 TOML 文本中提取 ccecho 需要的少量字段
// 这里使用轻量手写解析，只覆盖 model_provider 与 model_providers.* 相关键
func parseCodexConfig(raw []byte) CodexConfig {
	cfg := CodexConfig{
		ModelProviders: make(map[string]CodexProviderConfig),
	}

	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	currentProvider := ""
	for scanner.Scan() {
		line := trimTOMLLine(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentProvider = parseCodexProviderSection(line)
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = parseTOMLString(value)
		if currentProvider == "" {
			if key == "model_provider" {
				cfg.ModelProvider = value
			}
			continue
		}

		provider := cfg.ModelProviders[currentProvider]
		if key == "name" {
			provider.Name = value
		}
		if key == "base_url" {
			provider.BaseURL = value
		}
		if key == "wire_api" {
			provider.WireAPI = value
		}
		if key == "requires_openai_auth" {
			provider.RequiresOpenAIAuth = strings.EqualFold(value, "true")
		}
		cfg.ModelProviders[currentProvider] = provider
	}

	return cfg
}

// trimTOMLLine 去除一行 TOML 中的前后空白与行内注释
// 纯空行或纯注释行会被归一化为空字符串
func trimTOMLLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if idx := strings.Index(line, "#"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}

// parseCodexProviderSection 解析 [model_providers.<name>] 节标题并返回 <name>
// 非该前缀的小节会返回空字符串。
func parseCodexProviderSection(line string) string {
	section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
	const prefix = "model_providers."
	if !strings.HasPrefix(section, prefix) {
		return ""
	}
	return strings.Trim(section[len(prefix):], `"`)
}

// parseTOMLString 去掉 TOML 标量值外围的空白与双引号
func parseTOMLString(raw string) string {
	value := strings.TrimSpace(raw)
	return strings.Trim(value, `"`)
}
