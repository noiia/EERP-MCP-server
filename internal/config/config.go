// Package config handles YAML configuration loading for the MCP server.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds HTTP/transport settings.
type ServerConfig struct {
	// Transport is "sse", "streamable-http", or "stdio".
	Transport string `yaml:"transport"`
	// Address is the TCP address to listen on, e.g. ":8080".
	Address string `yaml:"address"`
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// DocsConfig points to the MkDocs project.
type DocsConfig struct {
	// Root is the directory containing the Markdown source files.
	Root string `yaml:"root"`
	// MkDocs is the path to mkdocs.yml.
	MkDocs string `yaml:"mkdocs"`
}

// SearchConfig tunes the search engine.
type SearchConfig struct {
	MaxResults int `yaml:"max_results"`
}

// Config is the top-level configuration structure.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Docs   DocsConfig   `yaml:"docs"`
	Search SearchConfig `yaml:"search"`
}

// Load reads and parses a YAML config file, applying sensible defaults.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	cfg := &Config{}
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

// Default returns a Config populated entirely with defaults.
func Default() *Config {
	cfg := &Config{}
	applyDefaults(cfg)
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Address == "" {
		cfg.Server.Address = ":8080"
	}
	if cfg.Server.Transport == "" {
		cfg.Server.Transport = "sse"
	}
	if cfg.Server.Name == "" {
		cfg.Server.Name = "eerp-docs-mcp"
	}
	if cfg.Server.Version == "" {
		cfg.Server.Version = "1.0.0"
	}
	if cfg.Docs.Root == "" {
		cfg.Docs.Root = "./docs"
	}
	if cfg.Docs.MkDocs == "" {
		cfg.Docs.MkDocs = "./mkdocs.yml"
	}
	if cfg.Search.MaxResults == 0 {
		cfg.Search.MaxResults = 10
	}
}
