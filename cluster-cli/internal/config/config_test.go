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
name = "test-cluster"

[ssh]
user = "pi"
key = "~/.ssh/id_test"

[dirs]
setup = "./cluster-setup"
platform = "./cluster-platform"

[setup]
default_inventory = "homelab"

[[nodes.agents]]
name = "rpi-3"
ip = "192.168.1.3"

[[nodes.agents]]
name = "rpi-4"
ip = "192.168.1.4"

[[nodes.servers]]
name = "rpi-1"
ip = "192.168.1.1"
role = "joining"

[[nodes.servers]]
name = "rpi-0"
ip = "192.168.1.0"
role = "init"
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
	assert.Equal(t, "pi", cfg.SSH.User)
	assert.Equal(t, "homelab", cfg.Setup.DefaultInventory)
	assert.Len(t, cfg.Nodes.Agents, 2)
	assert.Len(t, cfg.Nodes.Servers, 2)
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
