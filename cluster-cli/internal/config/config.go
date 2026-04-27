package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Cluster ClusterConfig `toml:"cluster"`
	SSH     SSHConfig     `toml:"ssh"`
	Dirs    DirsConfig    `toml:"dirs"`
	Setup   SetupConfig   `toml:"setup"`
	Nodes   NodesConfig   `toml:"nodes"`
}

type ClusterConfig struct {
	Name string `toml:"name"`
}

type SSHConfig struct {
	User string `toml:"user"`
	Key  string `toml:"key"`
}

type DirsConfig struct {
	Setup    string `toml:"setup"`
	Platform string `toml:"platform"`
}

type SetupConfig struct {
	DefaultInventory string `toml:"default_inventory"`
}

type NodesConfig struct {
	Agents  []Node `toml:"agents"`
	Servers []Node `toml:"servers"`
}

type Node struct {
	Name string `toml:"name"`
	IP   string `toml:"ip"`
	Role string `toml:"role"`
}

func (c *Config) Kubeconfig() string {
	return filepath.Join(home(), ".kube", "rpi-cluster.yaml")
}

func (c *Config) InitServer() Node {
	for _, s := range c.Nodes.Servers {
		if s.Role == "init" {
			return s
		}
	}
	return Node{}
}

func (c *Config) JoiningServers() []Node {
	var out []Node
	for _, s := range c.Nodes.Servers {
		if s.Role == "joining" {
			out = append(out, s)
		}
	}
	return out
}

func Load(path string) (*Config, error) {
	candidates := []string{path}
	if path == "" {
		candidates = defaultPaths()
	}

	for _, p := range candidates {
		p = expandHome(p)
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			var cfg Config
			if _, err := toml.DecodeFile(abs, &cfg); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", abs, err)
			}
			cfg.Dirs.Setup = expandHome(cfg.Dirs.Setup)
			cfg.Dirs.Platform = expandHome(cfg.Dirs.Platform)
			cfg.SSH.Key = expandHome(cfg.SSH.Key)
			return &cfg, nil
		}
	}
	return nil, fmt.Errorf("no config file found; tried: %s", strings.Join(candidates, ", "))
}

func defaultPaths() []string {
	return []string{
		"./config/cluster-config.toml",
		filepath.Join(home(), ".config", "rpicli", "config.toml"),
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home(), path[2:])
	}
	return path
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}
