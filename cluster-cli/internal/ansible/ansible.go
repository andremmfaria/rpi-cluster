package ansible

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	goansible "github.com/apenella/go-ansible/v2/pkg/playbook"
	"github.com/apenella/go-ansible/v2/pkg/execute"
)

type Playbook struct {
	dir     string
	file    string
	options *goansible.AnsiblePlaybookOptions
}

func NewPlaybook(dir, file string) *Playbook {
	return &Playbook{
		dir:     dir,
		file:    file,
		options: &goansible.AnsiblePlaybookOptions{},
	}
}

func (p *Playbook) WithInventory(inv string) *Playbook {
	if inv != "" {
		p.options.Inventory = fmt.Sprintf("inventories/%s/hosts.yml", inv)
	}
	return p
}

func (p *Playbook) WithLimit(limit string) *Playbook {
	p.options.Limit = limit
	return p
}

func (p *Playbook) WithTags(tags string) *Playbook {
	p.options.Tags = tags
	return p
}

func (p *Playbook) WithSkipTags(tags string) *Playbook {
	p.options.SkipTags = tags
	return p
}

func (p *Playbook) WithExtraVars(vars []string) *Playbook {
	for _, v := range vars {
		p.options.AddExtraVar(v, "")
	}
	return p
}

func (p *Playbook) WithCheckMode() *Playbook {
	p.options.Check = true
	p.options.Diff = true
	return p
}

func (p *Playbook) Run(ctx context.Context) error {
	exec := execute.NewDefaultExecute(
		execute.WithCmd(goansible.NewAnsiblePlaybookCmd(
			goansible.WithPlaybooks(p.file),
			goansible.WithPlaybookOptions(p.options),
		)),
		execute.WithCmdRunDir(p.dir),
		execute.WithWrite(os.Stdout),
		execute.WithWriteError(os.Stderr),
	)
	return exec.Execute(ctx)
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
