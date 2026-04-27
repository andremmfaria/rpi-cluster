package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfig  string
	setupDir    string
	platformDir string
)

var rootCmd = &cobra.Command{
	Use:   "rpicli",
	Short: "rpi-cluster management CLI",
	Long:  "Manage a 6-node Raspberry Pi k3s cluster — provisioning, platform, and operations.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	home, _ := os.UserHomeDir()

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", home+"/.kube/rpi-cluster.yaml", "Path to kubeconfig file")
	rootCmd.PersistentFlags().StringVar(&setupDir, "setup-dir", "./cluster-setup", "Path to cluster-setup directory")
	rootCmd.PersistentFlags().StringVar(&platformDir, "platform-dir", "./cluster-platform", "Path to cluster-platform directory")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(platformCmd)
	rootCmd.AddCommand(clusterCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("rpicli v0.1.0")
	},
}
