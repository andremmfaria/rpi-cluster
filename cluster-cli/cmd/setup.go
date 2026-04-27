package cmd

import (
	"context"

	"rpicli/internal/ansible"
	"rpicli/internal/printer"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Ansible — provision and bootstrap cluster nodes",
}

func init() {
	setupCmd.AddCommand(setupDepsCmd)
	setupCmd.AddCommand(setupPingCmd)
	setupCmd.AddCommand(setupDeployCmd)
	setupCmd.AddCommand(setupResetCmd)
	setupCmd.AddCommand(setupCheckCmd)
	setupCmd.AddCommand(setupLintCmd)
}

var setupDepsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Install required Ansible Galaxy collections",
	RunE: func(cmd *cobra.Command, args []string) error {
		printer.Header("Installing Ansible Galaxy collections")
		return ansible.RunGalaxy(cfg.Setup.Dir, "collection", "install", "community.general", "ansible.posix")
	},
}

var setupPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Pre-flight connectivity and hardware check",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := inventoryFlag(cmd)
		limit, _ := cmd.Flags().GetString("limit")
		printer.Header("Pre-flight validation — inventory: " + inv)
		return ansible.NewPlaybook(cfg.Setup.Dir, "playbooks/00-ping.yml").
			WithInventory(inv).
			WithLimit(limit).
			Run(context.Background())
	},
}

var setupDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Bootstrap the full cluster (site.yml)",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := inventoryFlag(cmd)
		limit, _ := cmd.Flags().GetString("limit")
		tags, _ := cmd.Flags().GetString("tags")
		extraVars, _ := cmd.Flags().GetStringArray("extra-var")
		printer.Header("Deploying cluster — inventory: " + inv)
		return ansible.NewPlaybook(cfg.Setup.Dir, "site.yml").
			WithInventory(inv).
			WithLimit(limit).
			WithTags(tags).
			WithExtraVars(extraVars).
			Run(context.Background())
	},
}

var setupResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Tear down k3s — DESTRUCTIVE, requires confirmation",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := inventoryFlag(cmd)
		printer.Warn("This will DESTROY the k3s cluster and wipe all node data.")
		printer.Warn("Target inventory: " + inv)
		if !printer.Confirm("Type 'yes' to confirm") {
			printer.Info("Aborted.")
			return nil
		}
		return ansible.NewPlaybook(cfg.Setup.Dir, "playbooks/99-reset.yml").
			WithInventory(inv).
			Run(context.Background())
	},
}

var setupCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Dry-run deploy (--check --diff)",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := inventoryFlag(cmd)
		limit, _ := cmd.Flags().GetString("limit")
		tags, _ := cmd.Flags().GetString("tags")
		printer.Header("Dry-run (check + diff) — inventory: " + inv)
		return ansible.NewPlaybook(cfg.Setup.Dir, "site.yml").
			WithInventory(inv).
			WithLimit(limit).
			WithTags(tags).
			WithCheckMode().
			Run(context.Background())
	},
}

var setupLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Run yamllint and ansible-lint",
	RunE: func(cmd *cobra.Command, args []string) error {
		printer.Header("Linting cluster-setup")
		if err := ansible.RunLint(cfg.Setup.Dir); err != nil {
			return err
		}
		printer.Success("All linters passed.")
		return nil
	},
}

func inventoryFlag(cmd *cobra.Command) string {
	inv, _ := cmd.Flags().GetString("inventory")
	if inv == "" {
		return cfg.Setup.DefaultInventory
	}
	return inv
}

func addSetupFlags(c *cobra.Command) {
	c.Flags().StringP("inventory", "i", "", "Inventory name under inventories/ (default: from config)")
	c.Flags().StringP("limit", "l", "", "Limit to hosts matching pattern")
	c.Flags().StringP("tags", "t", "", "Only run tasks with these tags")
	c.Flags().StringArrayP("extra-var", "e", nil, "Set extra variable (repeatable)")
}

func init() {
	for _, c := range []*cobra.Command{setupPingCmd, setupDeployCmd, setupResetCmd, setupCheckCmd} {
		addSetupFlags(c)
	}
}
