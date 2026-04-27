package ansible

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlaybookWithInventoryBuildsPath(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithInventory("homelab")
	assert.Equal(t, "inventories/homelab/hosts.yml", p.options.Inventory)
}

func TestPlaybookWithInventoryEmpty(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithInventory("")
	assert.Equal(t, "", p.options.Inventory)
}

func TestPlaybookWithLimit(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithLimit("rpi-0")
	assert.Equal(t, "rpi-0", p.options.Limit)
}

func TestPlaybookWithTags(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithTags("common,storage")
	assert.Equal(t, "common,storage", p.options.Tags)
}

func TestPlaybookWithSkipTags(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithSkipTags("slow")
	assert.Equal(t, "slow", p.options.SkipTags)
}

func TestPlaybookWithCheckMode(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").WithCheckMode()
	assert.True(t, p.options.Check)
	assert.True(t, p.options.Diff)
}

func TestPlaybookChaining(t *testing.T) {
	p := NewPlaybook("/repo", "site.yml").
		WithInventory("homelab").
		WithLimit("rpi-0").
		WithTags("common").
		WithCheckMode()

	assert.Equal(t, "inventories/homelab/hosts.yml", p.options.Inventory)
	assert.Equal(t, "rpi-0", p.options.Limit)
	assert.Equal(t, "common", p.options.Tags)
	assert.True(t, p.options.Check)
}
