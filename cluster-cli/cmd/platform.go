package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

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

		if server == "" {
			return fmt.Errorf("--server is required")
		}
		if user == "" {
			user = cfg.SSH.User
		}
		if key == "" {
			key = cfg.SSH.Key
		}
		if _, err := os.Stat(key); err != nil {
			return fmt.Errorf("SSH key not found: %s", key)
		}
		printer.Header("Fetching kubeconfig from " + user + "@" + server)
		return kube.FetchKubeconfig(server, user, key, out)
	},
}

func init() {
	platformKubeconfigCmd.Flags().String("server", "", "Control-plane node IP (required)")
	platformKubeconfigCmd.Flags().String("user", "", "SSH user (default: from config)")
	platformKubeconfigCmd.Flags().String("key", "", "SSH private key path (default: from config)")
	platformKubeconfigCmd.Flags().String("out", "", "Output path (default: from config kubeconfig)")
}

var platformApplyCmd = &cobra.Command{
	Use:   "apply <component>",
	Short: "Apply a component (kube-vip|metallb|ingress-nginx|longhorn|all)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return applyComponent(args[0])
	},
}

var platformDiffCmd = &cobra.Command{
	Use:   "diff <component>",
	Short: "Dry-run diff against live cluster",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return diffComponent(args[0])
	},
}

var platformDeleteCmd = &cobra.Command{
	Use:   "delete <component>",
	Short: "Remove a component — requires confirmation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		comp := args[0]
		printer.Warn("This will DELETE " + comp + " from the cluster.")
		if !printer.Confirm("Type 'yes' to confirm") {
			printer.Info("Aborted.")
			return nil
		}
		return deleteComponent(comp)
	},
}

func applyComponent(comp string) error {
	kubectl := func(args ...string) error {
		all := append([]string{"--kubeconfig=" + kubeconfig}, args...)
		c := exec.Command("kubectl", all...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}
	infraDir := cfg.Dirs.Platform

	switch comp {
	case "kube-vip":
		return kubectl("apply", "-f", infraDir+"/infrastructure/kube-vip/")
	case "metallb":
		if err := kubectl("apply", "-f", "https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml"); err != nil {
			return err
		}
		if err := waitForCRDs("ipaddresspools.metallb.io", "l2advertisements.metallb.io"); err != nil {
			return err
		}
		printer.Info("Waiting for MetalLB controller to be ready...")
		if err := kubectl("wait", "--for=condition=Available",
			"deployment/controller", "-n", "metallb-system", "--timeout=120s"); err != nil {
			return err
		}
		return kubectl("apply", "-f", infraDir+"/infrastructure/metallb/pool.yaml")
	case "ingress-nginx":
		return kubectl("apply", "-f", infraDir+"/infrastructure/ingress-nginx/")
	case "longhorn":
		_ = kubectl("apply", "-f", infraDir+"/infrastructure/longhorn/namespace.yaml")
		if err := kubectl("apply", "-f", infraDir+"/infrastructure/longhorn/helm-release.yaml"); err != nil {
			return err
		}
		printer.Info("Waiting for Longhorn CRDs (~2 min)...")
		if err := waitForCRDs("volumes.longhorn.io", "nodes.longhorn.io", "settings.longhorn.io"); err != nil {
			return err
		}
		return kubectl("apply", "-f", infraDir+"/infrastructure/longhorn/ingress.yaml")
	case "cert-manager":
		_ = kubectl("apply", "-f", infraDir+"/infrastructure/cert-manager/namespace.yaml")
		if err := kubectl("apply", "-f", infraDir+"/infrastructure/cert-manager/helm-release.yaml"); err != nil {
			return err
		}
		printer.Info("Waiting for cert-manager CRDs...")
		if err := waitForCRDs(
			"certificates.cert-manager.io",
			"clusterissuers.cert-manager.io",
			"issuers.cert-manager.io",
		); err != nil {
			return err
		}
		printer.Info("Waiting for cert-manager webhook to be ready...")
		if err := kubectl("wait", "--for=condition=Available",
			"deployment/cert-manager-webhook", "-n", "cert-manager", "--timeout=120s"); err != nil {
			return err
		}
		printer.Info("Applying ClusterIssuers...")
		_ = kubectl("apply", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-selfsigned.yaml")
		_ = kubectl("apply", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-letsencrypt-staging.yaml")
		return kubectl("apply", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-letsencrypt-prod.yaml")
	case "all":
		for _, c := range []string{"kube-vip", "metallb", "ingress-nginx", "longhorn", "cert-manager"} {
			printer.Header("Applying " + c)
			if err := applyComponent(c); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown component %q", comp)
	}
}

func waitForCRDs(crds ...string) error {
	args := []string{"--kubeconfig=" + kubeconfig, "wait", "--for=condition=Established", "--timeout=60s"}
	for _, crd := range crds {
		args = append(args, "crd/"+crd)
	}
	c := exec.Command("kubectl", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func diffComponent(comp string) error {
	kubectl := func(args ...string) error {
		all := append([]string{"--kubeconfig=" + kubeconfig}, args...)
		c := exec.Command("kubectl", all...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		_ = c.Run()
		return nil
	}
	infraDir := cfg.Dirs.Platform

	switch comp {
	case "kube-vip":
		return kubectl("diff", "-f", infraDir+"/infrastructure/kube-vip/")
	case "metallb":
		_ = kubectl("diff", "-f", "https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml")
		return kubectl("diff", "-f", infraDir+"/infrastructure/metallb/pool.yaml")
	case "ingress-nginx":
		return kubectl("diff", "-f", infraDir+"/infrastructure/ingress-nginx/")
	case "longhorn":
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/longhorn/namespace.yaml")
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/longhorn/helm-release.yaml")
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/longhorn/ingress.yaml")
		return nil
	case "cert-manager":
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/cert-manager/namespace.yaml")
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/cert-manager/helm-release.yaml")
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-selfsigned.yaml")
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-letsencrypt-staging.yaml")
		_ = kubectl("diff", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-letsencrypt-prod.yaml")
		return nil
	case "all":
		for _, c := range []string{"kube-vip", "metallb", "ingress-nginx", "longhorn", "cert-manager"} {
			_ = diffComponent(c)
		}
		return nil
	default:
		return fmt.Errorf("unknown component %q", comp)
	}
}

func deleteComponent(comp string) error {
	kubectl := func(args ...string) error {
		all := append([]string{"--kubeconfig=" + kubeconfig}, args...)
		c := exec.Command("kubectl", all...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}
	infraDir := cfg.Dirs.Platform

	switch comp {
	case "kube-vip":
		return kubectl("delete", "-f", infraDir+"/infrastructure/kube-vip/", "--ignore-not-found")

	case "metallb":
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/metallb/pool.yaml", "--ignore-not-found")
		return kubectl("delete", "-f", "https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml", "--ignore-not-found")

	case "ingress-nginx":
		return kubectl("delete", "-f", infraDir+"/infrastructure/ingress-nginx/", "--ignore-not-found")

	case "longhorn":
		printer.Info("Removing Longhorn admission webhooks...")
		_ = kubectl("delete", "validatingwebhookconfiguration", "longhorn-webhook-validator", "--ignore-not-found")
		_ = kubectl("delete", "mutatingwebhookconfiguration", "longhorn-webhook-mutator", "--ignore-not-found")

		_ = kubectl("delete", "-f", infraDir+"/infrastructure/longhorn/ingress.yaml", "--ignore-not-found")
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/longhorn/helm-release.yaml", "--ignore-not-found")

		if err := waitNamespaceTermination("longhorn-system", 180*time.Second); err != nil {
			printer.Warn("Namespace still terminating — force-finalizing...")
			forceFinalize("longhorn-system")
		}

		_ = kubectl("delete", "-f", infraDir+"/infrastructure/longhorn/namespace.yaml", "--ignore-not-found")

		printer.Info("Removing orphaned Longhorn CRDs...")
		removeLonghornCRDs()
		printer.Success("Longhorn deleted.")
		return nil

	case "cert-manager":
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-letsencrypt-prod.yaml", "--ignore-not-found")
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-letsencrypt-staging.yaml", "--ignore-not-found")
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/cert-manager/clusterissuer-selfsigned.yaml", "--ignore-not-found")
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/cert-manager/helm-release.yaml", "--ignore-not-found")
		_ = waitNamespaceTermination("cert-manager", 120*time.Second)
		_ = kubectl("delete", "-f", infraDir+"/infrastructure/cert-manager/namespace.yaml", "--ignore-not-found")
		printer.Success("cert-manager deleted.")
		return nil

	default:
		return fmt.Errorf("unknown component %q — valid: kube-vip, metallb, ingress-nginx, longhorn, cert-manager", comp)
	}
}

func waitNamespaceTermination(ns string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	c := exec.Command("kubectl", "--kubeconfig="+kubeconfig, "get", "namespace", ns)
	if c.Run() != nil {
		return nil
	}
	printer.Info("Waiting for " + ns + " namespace to terminate...")
	for time.Now().Before(deadline) {
		c := exec.Command("kubectl", "--kubeconfig="+kubeconfig, "get", "namespace", ns)
		if c.Run() != nil {
			return nil
		}
		time.Sleep(5 * time.Second)
		printer.Info("  Still waiting...")
	}
	return fmt.Errorf("timed out after %s", timeout)
}

func forceFinalize(ns string) {
	get := exec.Command("kubectl", "--kubeconfig="+kubeconfig, "get", "namespace", ns, "-o", "json")
	out, err := get.Output()
	if err != nil {
		return
	}
	patch := exec.Command("kubectl", "--kubeconfig="+kubeconfig, "replace", "--raw",
		"/api/v1/namespaces/"+ns+"/finalize", "-f", "-")
	patch.Stdin = stripFinalizers(out)
	patch.Stdout = os.Stdout
	_ = patch.Run()
}

func stripFinalizers(jsonData []byte) *os.File {
	f, _ := os.CreateTemp("", "ns-finalize-*.json")
	processed := removeFinalizersFromJSON(jsonData)
	_, _ = f.Write(processed)
	_, _ = f.Seek(0, 0)
	return f
}

func removeFinalizersFromJSON(data []byte) []byte {
	c := exec.Command("python3", "-c",
		`import sys,json; d=json.load(sys.stdin); d['spec']['finalizers']=[]; print(json.dumps(d))`)
	c.Stdin = os.Stdin
	_ = c
	return data
}

func removeLonghornCRDs() {
	list := exec.Command("kubectl", "--kubeconfig="+kubeconfig, "get", "crd", "-o", "name")
	out, err := list.Output()
	if err != nil || len(out) == 0 {
		return
	}
	grep := exec.Command("grep", ".longhorn.io")
	grep.Stdin = func() *os.File { f, _ := os.Open(os.DevNull); return f }()
	_ = grep

	xargs := exec.Command("sh", "-c",
		`kubectl --kubeconfig=`+kubeconfig+` get crd -o name | grep '\.longhorn\.io' | xargs kubectl --kubeconfig=`+kubeconfig+` delete --ignore-not-found`)
	xargs.Stdout = os.Stdout
	xargs.Stderr = os.Stderr
	_ = xargs.Run()
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
		c.Dir = cfg.Dirs.Platform
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("yamllint failed")
		}
		printer.Success("yamllint passed.")
		return nil
	},
}
