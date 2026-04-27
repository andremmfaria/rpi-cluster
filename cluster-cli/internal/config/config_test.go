package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTOML = `
[cluster]
name  = "test-cluster"
email = "test@example.com"

[ssh]
user = "pi"
key  = "~/.ssh/id_test"

[[nodes]]
name = "rpi-0"
ip   = "192.168.1.0"
role = "init"

[[nodes]]
name = "rpi-1"
ip   = "192.168.1.1"
role = "joining"

[[nodes]]
name = "rpi-2"
ip   = "192.168.1.2"
role = "agent"

[[nodes]]
name = "rpi-3"
ip   = "192.168.1.3"
role = "agent"

[setup]
dir               = "./cluster-setup"
default_inventory = "homelab"

[setup.k3s]
version      = "v1.35.3+k3s1"
token        = "secret"
cluster_cidr = "10.42.0.0/16"
service_cidr = "10.43.0.0/16"
data_dir     = "/mnt/nvme/k3s"
api_vip      = "192.168.1.30"

[setup.storage]
disk   = "/dev/sda"
device = "/dev/sda1"
mount  = "/mnt/nvme"
fstype = "ext4"

[platform]
dir = "./cluster-platform"

[platform.network]
vip          = "192.168.1.30"
ingress_ip   = "192.168.1.40"
metallb_pool = "192.168.1.40-192.168.1.60"

[platform.dns]
longhorn = "longhorn.test.srv"
rancher  = "rancher.test.srv"
grafana  = "grafana.test.srv"

[platform.versions]
kube_vip            = "v1.1.2"
metallb             = "v0.15.3"
ingress_nginx_chart = "4.15.1"
longhorn            = "1.11.1"
cert_manager        = "v1.20.2"
`

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "rpicli*.toml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestLoadConfig(t *testing.T) {
	path := writeTempConfig(t, testTOML)
	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "test-cluster", cfg.Cluster.Name)
	assert.Equal(t, "test@example.com", cfg.Cluster.Email)
	assert.Equal(t, "pi", cfg.SSH.User)
	assert.Len(t, cfg.Nodes, 4)
	assert.Equal(t, "homelab", cfg.Setup.DefaultInventory)
	assert.Equal(t, "v1.35.3+k3s1", cfg.Setup.K3s.Version)
	assert.Equal(t, "/dev/sda", cfg.Setup.Storage.Disk)
	assert.Equal(t, "192.168.1.30", cfg.Platform.Network.VIP)
	assert.Equal(t, "longhorn.test.srv", cfg.Platform.DNS.Longhorn)
	assert.Equal(t, "v1.1.2", cfg.Platform.Versions.KubeVIP)
	assert.Equal(t, "v1.20.2", cfg.Platform.Versions.CertManager)
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config file found")
}

func TestLoadConfigBadTOML(t *testing.T) {
	path := writeTempConfig(t, "not valid toml [[[")
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

func TestNodesByRole(t *testing.T) {
	path := writeTempConfig(t, testTOML)
	cfg, err := Load(path)
	require.NoError(t, err)

	agents := cfg.Agents()
	require.Len(t, agents, 2)
	assert.Equal(t, "rpi-2", agents[0].Name)
	assert.Equal(t, "192.168.1.2", agents[0].IP)
}

func TestInitServer(t *testing.T) {
	path := writeTempConfig(t, testTOML)
	cfg, err := Load(path)
	require.NoError(t, err)

	init := cfg.InitServer()
	assert.Equal(t, "rpi-0", init.Name)
	assert.Equal(t, "192.168.1.0", init.IP)
}

func TestJoiningServers(t *testing.T) {
	path := writeTempConfig(t, testTOML)
	cfg, err := Load(path)
	require.NoError(t, err)

	joining := cfg.JoiningServers()
	require.Len(t, joining, 1)
	assert.Equal(t, "rpi-1", joining[0].Name)
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := expandHome("~/foo/bar")
	assert.Equal(t, filepath.Join(home, "foo", "bar"), result)
}

func TestExpandHomeNoTilde(t *testing.T) {
	assert.Equal(t, "/absolute/path", expandHome("/absolute/path"))
}

func TestAllNodes(t *testing.T) {
	path := writeTempConfig(t, testTOML)
	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Len(t, cfg.Nodes, 4)
	assert.Equal(t, "rpi-0", cfg.Nodes[0].Name)
	assert.Equal(t, "rpi-3", cfg.Nodes[3].Name)
}
