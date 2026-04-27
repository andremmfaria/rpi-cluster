package cmd

import (
	"fmt"
	"os"

	"rpicli/internal/config"

	"github.com/spf13/cobra"
)

var (
	configPath string
	cfg        *config.Config
	kubeconfig string
)

var rootCmd = &cobra.Command{
	Use:   "rpicli",
	Short: "rpi-cluster management CLI",
	Long:  "Manage a Raspberry Pi k3s cluster — provisioning, platform, and operations.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		if kubeconfig == "" {
			kubeconfig = cfg.Kubeconfig()
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	home, _ := os.UserHomeDir()
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Config file path (default: ./rpicli.toml or ~/.config/rpicli/config.toml)")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", home+"/.kube/rpi-cluster.yaml", "Path to kubeconfig file")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(platformCmd)
	rootCmd.AddCommand(clusterCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("rpicli v0.1.0")
	},
}
