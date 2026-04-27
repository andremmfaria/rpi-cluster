package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Cluster  ClusterConfig  `toml:"cluster"`
	SSH      SSHConfig      `toml:"ssh"`
	Nodes    []Node         `toml:"nodes"`
	Setup    SetupConfig    `toml:"setup"`
	Platform PlatformConfig `toml:"platform"`
}

type ClusterConfig struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
}

type SSHConfig struct {
	User string `toml:"user"`
	Key  string `toml:"key"`
}

type Node struct {
	Name string `toml:"name"`
	IP   string `toml:"ip"`
	Role string `toml:"role"`
}

type SetupConfig struct {
	Dir              string         `toml:"dir"`
	DefaultInventory string         `toml:"default_inventory"`
	K3s              K3sConfig      `toml:"k3s"`
	Storage          StorageConfig  `toml:"storage"`
}

type K3sConfig struct {
	Version     string `toml:"version"`
	Token       string `toml:"token"`
	ClusterCIDR string `toml:"cluster_cidr"`
	ServiceCIDR string `toml:"service_cidr"`
	DataDir     string `toml:"data_dir"`
	APIVIP      string `toml:"api_vip"`
}

type StorageConfig struct {
	Disk   string `toml:"disk"`
	Device string `toml:"device"`
	Mount  string `toml:"mount"`
	FSType string `toml:"fstype"`
}

type PlatformConfig struct {
	Dir      string           `toml:"dir"`
	Network  NetworkConfig    `toml:"network"`
	DNS      DNSConfig        `toml:"dns"`
	Versions VersionsConfig   `toml:"versions"`
}

type NetworkConfig struct {
	VIP         string `toml:"vip"`
	IngressIP   string `toml:"ingress_ip"`
	MetalLBPool string `toml:"metallb_pool"`
}

type DNSConfig struct {
	Longhorn string `toml:"longhorn"`
	Rancher  string `toml:"rancher"`
	Grafana  string `toml:"grafana"`
}

type VersionsConfig struct {
	KubeVIP           string `toml:"kube_vip"`
	MetalLB           string `toml:"metallb"`
	IngressNginxChart string `toml:"ingress_nginx_chart"`
	Longhorn          string `toml:"longhorn"`
	CertManager       string `toml:"cert_manager"`
}

func (c *Config) Kubeconfig() string {
	return filepath.Join(home(), ".kube", "rpi-cluster.yaml")
}

func (c *Config) NodesByRole(role string) []Node {
	var out []Node
	for _, n := range c.Nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

func (c *Config) InitServer() Node {
	for _, n := range c.Nodes {
		if n.Role == "init" {
			return n
		}
	}
	return Node{}
}

func (c *Config) JoiningServers() []Node {
	return c.NodesByRole("joining")
}

func (c *Config) Agents() []Node {
	return c.NodesByRole("agent")
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
			cfg.Setup.Dir = expandHome(cfg.Setup.Dir)
			cfg.Platform.Dir = expandHome(cfg.Platform.Dir)
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
