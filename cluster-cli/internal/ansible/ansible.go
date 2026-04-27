package ansible

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Playbook struct {
	dir       string
	playbook  string
	inventory string
	limit     string
	tags      string
	extraVars []string
	checkMode bool
}

func NewPlaybook(dir, playbook string) *Playbook {
	return &Playbook{dir: dir, playbook: playbook, inventory: "homelab"}
}

func (p *Playbook) WithInventory(inv string) *Playbook {
	if inv != "" {
		p.inventory = inv
	}
	return p
}

func (p *Playbook) WithLimit(limit string) *Playbook {
	p.limit = limit
	return p
}

func (p *Playbook) WithTags(tags string) *Playbook {
	p.tags = tags
	return p
}

func (p *Playbook) WithExtraVars(vars []string) *Playbook {
	p.extraVars = vars
	return p
}

func (p *Playbook) WithCheckMode() *Playbook {
	p.checkMode = true
	return p
}

func (p *Playbook) Args() []string {
	inventoryPath := fmt.Sprintf("inventories/%s/hosts.yml", p.inventory)
	args := []string{"-i", inventoryPath, p.playbook}

	if p.checkMode {
		args = append(args, "--check", "--diff")
	}
	if p.limit != "" {
		args = append(args, "-l", p.limit)
	}
	if p.tags != "" {
		args = append(args, "-t", p.tags)
	}
	for _, v := range p.extraVars {
		args = append(args, "-e", v)
	}
	return args
}

func (p *Playbook) Run() error {
	args := p.Args()
	fmt.Println("Running: ansible-playbook " + strings.Join(args, " "))
	fmt.Println()

	c := exec.Command("ansible-playbook", args...)
	c.Dir = p.dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func RunGalaxy(dir string, args ...string) error {
	c := exec.Command("ansible-galaxy", args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func RunLint(dir string) error {
	for _, tool := range [][]string{
		{"yamllint", "."},
		{"ansible-lint"},
	} {
		c := exec.Command(tool[0], tool[1:]...)
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s failed", tool[0])
		}
	}
	return nil
}
