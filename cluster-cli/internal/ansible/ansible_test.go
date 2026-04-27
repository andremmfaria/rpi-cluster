package ansible

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaybookDefaultArgs(t *testing.T) {
	p := NewPlaybook("/repo/cluster-setup", "site.yml")
	args := p.Args()

	require.Contains(t, args, "site.yml")
	require.Contains(t, args, "-i")
	assert.Contains(t, args, "inventories/homelab/hosts.yml")
}

func TestPlaybookWithInventory(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithInventory("staging")
	args := p.Args()

	assert.Contains(t, args, "inventories/staging/hosts.yml")
}

func TestPlaybookWithLimit(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithLimit("rpi-0")
	args := p.Args()

	assert.Contains(t, args, "-l")
	assert.Contains(t, args, "rpi-0")
}

func TestPlaybookWithTags(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithTags("common,storage")
	args := p.Args()

	assert.Contains(t, args, "-t")
	assert.Contains(t, args, "common,storage")
}

func TestPlaybookWithCheckMode(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithCheckMode()
	args := p.Args()

	assert.Contains(t, args, "--check")
	assert.Contains(t, args, "--diff")
}

func TestPlaybookWithExtraVars(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithExtraVars([]string{"k3s_version=v1.35.3+k3s1", "foo=bar"})
	args := p.Args()

	assert.Contains(t, args, "-e")
	assert.Contains(t, args, "k3s_version=v1.35.3+k3s1")
	assert.Contains(t, args, "foo=bar")
}

func TestPlaybookNoLimitWhenEmpty(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithLimit("")
	args := p.Args()

	assert.NotContains(t, args, "-l")
}

func TestPlaybookChaining(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").
		WithInventory("homelab").
		WithLimit("rpi-0").
		WithTags("common").
		WithCheckMode()

	args := p.Args()
	assert.Contains(t, args, "--check")
	assert.Contains(t, args, "-l")
	assert.Contains(t, args, "rpi-0")
	assert.Contains(t, args, "-t")
	assert.Contains(t, args, "common")
}
