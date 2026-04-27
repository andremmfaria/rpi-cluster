package cmd

import (
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
		return ansible.RunGalaxy(setupDir, "collection", "install", "community.general", "ansible.posix")
	},
}

var setupPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Pre-flight connectivity and hardware check",
	RunE: func(cmd *cobra.Command, args []string) error {
		inventory, _ := cmd.Flags().GetString("inventory")
		printer.Header("Pre-flight validation — inventory: " + inventory)
		return ansible.NewPlaybook(setupDir, "playbooks/00-ping.yml").
			WithInventory(inventory).
			Run()
	},
}

var setupDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Bootstrap the full cluster (site.yml)",
	RunE: func(cmd *cobra.Command, args []string) error {
		inventory, _ := cmd.Flags().GetString("inventory")
		limit, _ := cmd.Flags().GetString("limit")
		tags, _ := cmd.Flags().GetString("tags")
		extraVars, _ := cmd.Flags().GetStringArray("extra-var")
		printer.Header("Deploying cluster — inventory: " + inventory)
		return ansible.NewPlaybook(setupDir, "site.yml").
			WithInventory(inventory).
			WithLimit(limit).
			WithTags(tags).
			WithExtraVars(extraVars).
			Run()
	},
}

var setupResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Tear down k3s — DESTRUCTIVE, requires confirmation",
	RunE: func(cmd *cobra.Command, args []string) error {
		inventory, _ := cmd.Flags().GetString("inventory")
		printer.Warn("This will DESTROY the k3s cluster and wipe all node data.")
		printer.Warn("Target inventory: " + inventory)
		if !printer.Confirm("Type 'yes' to confirm") {
			printer.Info("Aborted.")
			return nil
		}
		return ansible.NewPlaybook(setupDir, "playbooks/99-reset.yml").
			WithInventory(inventory).
			Run()
	},
}

var setupCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Dry-run deploy (--check --diff)",
	RunE: func(cmd *cobra.Command, args []string) error {
		inventory, _ := cmd.Flags().GetString("inventory")
		printer.Header("Dry-run (check + diff) — inventory: " + inventory)
		return ansible.NewPlaybook(setupDir, "site.yml").
			WithInventory(inventory).
			WithCheckMode().
			Run()
	},
}

var setupLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Run yamllint and ansible-lint",
	RunE: func(cmd *cobra.Command, args []string) error {
		printer.Header("Linting cluster-setup")
		if err := ansible.RunLint(setupDir); err != nil {
			return err
		}
		printer.Success("All linters passed.")
		return nil
	},
}

func addSetupFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("inventory", "i", "homelab", "Inventory name under inventories/")
	cmd.Flags().StringP("limit", "l", "", "Limit to hosts matching pattern")
	cmd.Flags().StringP("tags", "t", "", "Only run tasks with these tags")
	cmd.Flags().StringArrayP("extra-var", "e", nil, "Set extra variable (repeatable)")
}

func init() {
	for _, c := range []*cobra.Command{setupPingCmd, setupDeployCmd, setupResetCmd, setupCheckCmd} {
		addSetupFlags(c)
	}
}
