package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig            `json:"server"`
	Auth     AuthConfig              `json:"auth"`
	Store    StoreConfig             `json:"store"`
	Providers map[string]ProviderEntry `json:"providers"`
	Protocols ProtocolsConfig        `json:"protocols"`
	Logging  LoggingConfig           `json:"logging"`
	Health   HealthConfig            `json:"health"`
}

type ServerConfig struct {
	Host      string            `json:"host"`
	Port      int               `json:"port"`
	WebSocket WebSocketConfig   `json:"websocket"`
	CORS      CORSConfig        `json:"cors"`
}

type WebSocketConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type CORSConfig struct {
	AllowedOrigins []string `json:"allowedOrigins"`
}

type AuthConfig struct {
	JWT     JWTConfig    `json:"jwt"`
	APIKeys []APIKeyEntry `json:"apiKeys"`
}

type JWTConfig struct {
	Secret string        `json:"secret"`
	Expiry time.Duration `json:"expiry"`
}

type APIKeyEntry struct {
	Name   string   `json:"name"`
	Key    string   `json:"key"`
	Scopes []string `json:"scopes"`
}

type StoreConfig struct {
	Type   string       `json:"type"`
	SQLite SQLiteConfig `json:"sqlite"`
}

type SQLiteConfig struct {
	Path string `json:"path"`
}

type ProviderEntry struct {
	Enabled bool              `json:"enabled"`
	Endpoint string           `json:"endpoint"`
	Auth   ProviderAuthEntry  `json:"auth"`
	Options map[string]any    `json:"options"`
}

type ProviderAuthEntry struct {
	Token  string `json:"token,omitempty"`
	APIKey string `json:"apiKey,omitempty"`
}

type ProtocolsConfig struct {
	A2A    ProtocolEntry `json:"a2a"`
	ACP    ProtocolEntry `json:"acp"`
	MCP    ProtocolEntry `json:"mcp"`
	OpenAI OpenAIConfig  `json:"openai"`
}

type ProtocolEntry struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type OpenAIConfig struct {
	Enabled         bool   `json:"enabled"`
	ChatCompletions string `json:"chatCompletions"`
	Responses       string `json:"responses"`
}

type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

type HealthConfig struct {
	CheckInterval       time.Duration `json:"checkInterval"`
	UnhealthyThreshold  int           `json:"unhealthyThreshold"`
}

func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.websocket.enabled", true)
	v.SetDefault("server.websocket.path", "/ws")
	v.SetDefault("store.type", "sqlite")
	v.SetDefault("store.sqlite.path", "./data/gateway.db")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("health.checkInterval", "30s")
	v.SetDefault("health.unhealthyThreshold", 3)
	v.SetDefault("protocols.a2a.enabled", true)
	v.SetDefault("protocols.a2a.path", "/a2a")
	v.SetDefault("protocols.acp.enabled", true)
	v.SetDefault("protocols.acp.path", "/acp")
	v.SetDefault("protocols.mcp.enabled", true)
	v.SetDefault("protocols.mcp.path", "/mcp")
	v.SetDefault("protocols.openai.enabled", true)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("gateway")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) DataDir() string {
	dir := "./data"
	if c.Store.Type == "sqlite" && c.Store.SQLite.Path != "" {
		dir = c.Store.SQLite.Path
		for i := len(dir) - 1; i >= 0; i-- {
			if dir[i] == '/' || dir[i] == '\\' {
				dir = dir[:i]
				break
			}
		}
	}
	os.MkdirAll(dir, 0755)
	return dir
}
