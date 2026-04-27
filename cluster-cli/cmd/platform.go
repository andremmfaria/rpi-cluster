package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"rpicli/internal/component"
	"rpicli/internal/kube"
	"rpicli/internal/printer"

	"github.com/spf13/cobra"
)

var platformCmd = &cobra.Command{
	Use:   "platform",
	Short: "kubectl — manage cluster infrastructure",
}

func init() {
	platformCmd.AddCommand(platformKubeconfigCmd)
	platformCmd.AddCommand(platformApplyCmd)
	platformCmd.AddCommand(platformDiffCmd)
	platformCmd.AddCommand(platformDeleteCmd)
	platformCmd.AddCommand(platformStatusCmd)
	platformCmd.AddCommand(platformLogsCmd)
	platformCmd.AddCommand(platformEventsCmd)
	platformCmd.AddCommand(platformRestartCmd)
	platformCmd.AddCommand(platformDescribeCmd)
	platformCmd.AddCommand(platformExecCmd)
	platformCmd.AddCommand(platformLintCmd)
}

func newKubeClient() (kube.Interface, error) {
	return kube.NewClient(kubeconfig)
}

var platformKubeconfigCmd = &cobra.Command{
	Use:   "kubeconfig",
	Short: "Fetch kubeconfig from a control-plane node",
	RunE: func(cmd *cobra.Command, args []string) error {
		server, _ := cmd.Flags().GetString("server")
		user, _ := cmd.Flags().GetString("user")
		key, _ := cmd.Flags().GetString("key")
		out, _ := cmd.Flags().GetString("out")

		if server == "" || user == "" || key == "" {
			return fmt.Errorf("--server, --user, and --key are required")
		}
		if _, err := os.Stat(key); err != nil {
			return fmt.Errorf("SSH key not found: %s", key)
		}
		printer.Header("Fetching kubeconfig from " + user + "@" + server)
		return kube.FetchKubeconfig(server, user, key, out)
	},
}

func init() {
	home, _ := os.UserHomeDir()
	platformKubeconfigCmd.Flags().String("server", "", "Control-plane node IP (required)")
	platformKubeconfigCmd.Flags().String("user", "", "SSH user (required)")
	platformKubeconfigCmd.Flags().String("key", "", "SSH private key path (required)")
	platformKubeconfigCmd.Flags().String("out", home+"/.kube/rpi-cluster.yaml", "Output path")
}

var platformApplyCmd = &cobra.Command{
	Use:   "apply <component>",
	Short: "Apply a component (kube-vip|metallb|ingress-nginx|longhorn|all)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return kubectlInDir(platformDir, "apply", args[0])
	},
}

var platformDiffCmd = &cobra.Command{
	Use:   "diff <component>",
	Short: "Dry-run diff against live cluster",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return kubectlInDir(platformDir, "diff", args[0])
	},
}

var platformDeleteCmd = &cobra.Command{
	Use:   "delete <component>",
	Short: "Remove a component — requires confirmation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printer.Warn("This will DELETE " + args[0] + " from the cluster.")
		if !printer.Confirm("Type 'yes' to confirm") {
			printer.Info("Aborted.")
			return nil
		}
		return kubectlInDir(platformDir, "delete", args[0])
	},
}

func kubectlInDir(dir, action, comp string) error {
	c := exec.Command("kubectl", "--kubeconfig="+kubeconfig, action, comp)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

var platformStatusCmd = &cobra.Command{
	Use:   "status [component]",
	Short: "Show pod/service status (kube-vip|metallb|ingress-nginx|longhorn|all)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := "all"
		if len(args) > 0 {
			name = args[0]
		}
		client, err := newKubeClient()
		if err != nil {
			return err
		}
		return kube.PrintStatus(context.Background(), client, name)
	},
}

var platformLogsCmd = &cobra.Command{
	Use:   "logs <component|pod>",
	Short: "Stream logs — resolves component names to namespace/selector",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tail, _ := cmd.Flags().GetInt64("tail")
		follow, _ := cmd.Flags().GetBool("follow")
		namespace, _ := cmd.Flags().GetString("namespace")
		container, _ := cmd.Flags().GetString("container")

		client, err := newKubeClient()
		if err != nil {
			return err
		}
		return kube.PrintLogs(context.Background(), client, kube.LogOptions{
			Target:    args[0],
			Namespace: namespace,
			Container: container,
			Tail:      tail,
			Follow:    follow,
		}, os.Stdout)
	},
}

func init() {
	platformLogsCmd.Flags().Int64("tail", 100, "Number of recent log lines to show")
	platformLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	platformLogsCmd.Flags().StringP("namespace", "n", "", "Namespace (required for direct pod name)")
	platformLogsCmd.Flags().StringP("container", "c", "", "Container name")
}

var platformEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show warning events sorted by timestamp",
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace, _ := cmd.Flags().GetString("namespace")
		all, _ := cmd.Flags().GetBool("all-namespaces")
		client, err := newKubeClient()
		if err != nil {
			return err
		}
		return kube.PrintEvents(context.Background(), client, namespace, all)
	},
}

func init() {
	platformEventsCmd.Flags().StringP("namespace", "n", "", "Namespace (default: all)")
	platformEventsCmd.Flags().BoolP("all-namespaces", "A", false, "Show events from all namespaces")
}

var platformRestartCmd = &cobra.Command{
	Use:   "restart <component>",
	Short: "Rollout restart a component's pods",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		comp, err := component.Get(args[0])
		if err != nil {
			return err
		}
		client, err := newKubeClient()
		if err != nil {
			return err
		}
		printer.Header("Restarting " + comp.Name)
		return kube.RolloutRestart(context.Background(), client, comp)
	},
}

var platformDescribeCmd = &cobra.Command{
	Use:   "describe <resource-type> [name]",
	Short: "Describe a Kubernetes resource",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace, _ := cmd.Flags().GetString("namespace")
		kubectlArgs := []string{"--kubeconfig=" + kubeconfig, "describe"}
		if namespace != "" {
			kubectlArgs = append(kubectlArgs, "-n", namespace)
		}
		kubectlArgs = append(kubectlArgs, args...)
		c := exec.Command("kubectl", kubectlArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	platformDescribeCmd.Flags().StringP("namespace", "n", "", "Namespace")
}

var platformExecCmd = &cobra.Command{
	Use:   "exec <component|pod> [-- command]",
	Short: "Exec into a pod (default: sh)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace, _ := cmd.Flags().GetString("namespace")
		client, err := newKubeClient()
		if err != nil {
			return err
		}

		target := args[0]
		execCmd := []string{"sh"}
		if cmd.ArgsLenAtDash() >= 0 {
			execCmd = args[cmd.ArgsLenAtDash():]
		}

		pod, ns, err := kube.ResolvePod(context.Background(), client, target, namespace)
		if err != nil {
			return err
		}

		kubectlArgs := []string{"--kubeconfig=" + kubeconfig, "exec", "-n", ns, "-it", pod, "--"}
		kubectlArgs = append(kubectlArgs, execCmd...)
		c := exec.Command("kubectl", kubectlArgs...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	platformExecCmd.Flags().StringP("namespace", "n", "", "Namespace (required for direct pod name)")
}

var platformLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Run yamllint on cluster-platform manifests",
	RunE: func(cmd *cobra.Command, args []string) error {
		printer.Header("Linting cluster-platform manifests")
		c := exec.Command("yamllint", ".")
		c.Dir = platformDir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("yamllint failed")
		}
		printer.Success("yamllint passed.")
		return nil
	},
}
