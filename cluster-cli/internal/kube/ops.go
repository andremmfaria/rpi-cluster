package kube

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"rpicli/internal/component"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Interface = kubernetes.Interface

func NewClient(kubeconfigPath string) (kubernetes.Interface, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig from %s: %w", kubeconfigPath, err)
	}
	return kubernetes.NewForConfig(cfg)
}

func PrintNodes(ctx context.Context, client Interface) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("%-10s %-30s %-10s\n", "NAME", "STATUS", "VERSION")
	for _, n := range nodes.Items {
		status := nodeStatus(n)
		fmt.Printf("%-10s %-30s %-10s\n", n.Name, status, n.Status.NodeInfo.KubeletVersion)
	}
	return nil
}

func nodeStatus(n corev1.Node) string {
	parts := []string{}
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			parts = append(parts, "Ready")
		}
	}
	if n.Spec.Unschedulable {
		parts = append(parts, "SchedulingDisabled")
	}
	if len(parts) == 0 {
		return "NotReady"
	}
	return strings.Join(parts, ",")
}

func PrintStatus(ctx context.Context, client Interface, name string) error {
	if name == "all" {
		for _, comp := range component.All() {
			fmt.Println()
			if err := printComponentStatus(ctx, client, comp); err != nil {
				return err
			}
		}
		return nil
	}
	comp, err := component.Get(name)
	if err != nil {
		return err
	}
	fmt.Println()
	return printComponentStatus(ctx, client, comp)
}

func printComponentStatus(ctx context.Context, client Interface, comp component.Component) error {
	fmt.Printf("==> %s (%s)\n", comp.Name, comp.Namespace)
	pods, err := client.CoreV1().Pods(comp.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: comp.Selector,
	})
	if err != nil {
		fmt.Println("  not found")
		return nil
	}
	if len(pods.Items) == 0 {
		fmt.Println("  no pods found")
		return nil
	}
	fmt.Printf("  %-50s %-8s %-10s\n", "NAME", "READY", "STATUS")
	for _, p := range pods.Items {
		ready := fmt.Sprintf("%d/%d", readyContainers(p), len(p.Spec.Containers))
		fmt.Printf("  %-50s %-8s %-10s\n", p.Name, ready, string(p.Status.Phase))
	}
	return nil
}

func readyContainers(p corev1.Pod) int {
	count := 0
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			count++
		}
	}
	return count
}

type LogOptions struct {
	Target    string
	Namespace string
	Container string
	Tail      int64
	Follow    bool
}

func PrintLogs(ctx context.Context, client Interface, opts LogOptions, w io.Writer) error {
	pods, ns, err := resolvePods(ctx, client, opts.Target, opts.Namespace)
	if err != nil {
		return err
	}

	logOpts := &corev1.PodLogOptions{
		Container: opts.Container,
		Follow:    opts.Follow,
	}
	if opts.Tail > 0 {
		logOpts.TailLines = &opts.Tail
	}

	for _, pod := range pods {
		fmt.Fprintf(w, "\n=== %s ===\n", pod)
		req := client.CoreV1().Pods(ns).GetLogs(pod, logOpts)
		stream, err := req.Stream(ctx)
		if err != nil {
			fmt.Fprintf(w, "  error: %v\n", err)
			continue
		}
		_, _ = io.Copy(w, stream)
		stream.Close()
	}
	return nil
}

func PrintEvents(ctx context.Context, client Interface, namespace string, allNamespaces bool) error {
	ns := namespace
	if allNamespaces || ns == "" {
		ns = ""
	}

	events, err := client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "type!=Normal",
	})
	if err != nil {
		return err
	}

	items := events.Items
	sortEventsByTime(items)

	if len(items) == 0 {
		fmt.Println("No warning events found.")
		return nil
	}

	fmt.Printf("%-12s %-9s %-25s %-45s %s\n", "LAST SEEN", "TYPE", "REASON", "OBJECT", "MESSAGE")
	for _, e := range items {
		age := formatAge(e.LastTimestamp.Time)
		obj := fmt.Sprintf("%s/%s", strings.ToLower(e.InvolvedObject.Kind), e.InvolvedObject.Name)
		msg := e.Message
		if len(msg) > 80 {
			msg = msg[:77] + "..."
		}
		fmt.Printf("%-12s %-9s %-25s %-45s %s\n", age, e.Type, e.Reason, obj, msg)
	}
	return nil
}

func sortEventsByTime(events []corev1.Event) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].LastTimestamp.Before(&events[j-1].LastTimestamp); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func ResolvePod(ctx context.Context, client Interface, target, namespace string) (pod, ns string, err error) {
	pods, resolvedNS, err := resolvePods(ctx, client, target, namespace)
	if err != nil {
		return "", "", err
	}
	return pods[0], resolvedNS, nil
}

func resolvePods(ctx context.Context, client Interface, target, namespace string) ([]string, string, error) {
	comp, compErr := component.Get(target)
	if compErr == nil {
		pods, err := client.CoreV1().Pods(comp.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: comp.Selector,
		})
		if err != nil || len(pods.Items) == 0 {
			return nil, "", fmt.Errorf("no pods found for component %q", target)
		}
		names := make([]string, len(pods.Items))
		for i, p := range pods.Items {
			names[i] = p.Name
		}
		return names, comp.Namespace, nil
	}

	if namespace == "" {
		return nil, "", fmt.Errorf("-n/--namespace is required when specifying a pod name directly")
	}
	return []string{target}, namespace, nil
}

func RolloutRestart(ctx context.Context, client Interface, comp component.Component) error {
	now := time.Now().UTC().Format(time.RFC3339)
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`, now)

	for _, w := range comp.Workloads {
		switch w.Kind {
		case "deployment":
			_, err := client.AppsV1().Deployments(comp.Namespace).Patch(
				ctx, w.Name, "application/merge-patch+json", []byte(patch), metav1.PatchOptions{},
			)
			if err != nil {
				return fmt.Errorf("restarting deployment %s: %w", w.Name, err)
			}
		case "daemonset":
			_, err := client.AppsV1().DaemonSets(comp.Namespace).Patch(
				ctx, w.Name, "application/merge-patch+json", []byte(patch), metav1.PatchOptions{},
			)
			if err != nil {
				return fmt.Errorf("restarting daemonset %s: %w", w.Name, err)
			}
		}
		fmt.Printf("  restarted %s/%s\n", w.Kind, w.Name)
	}
	return nil
}

func CordonNode(ctx context.Context, client Interface, node string) error {
	n, err := client.CoreV1().Nodes().Get(ctx, node, metav1.GetOptions{})
	if err != nil {
		return err
	}
	n.Spec.Unschedulable = true
	_, err = client.CoreV1().Nodes().Update(ctx, n, metav1.UpdateOptions{})
	return err
}

func UncordonNode(ctx context.Context, client Interface, node string) error {
	n, err := client.CoreV1().Nodes().Get(ctx, node, metav1.GetOptions{})
	if err != nil {
		return err
	}
	n.Spec.Unschedulable = false
	_, err = client.CoreV1().Nodes().Update(ctx, n, metav1.UpdateOptions{})
	return err
}

func DrainNode(ctx context.Context, client Interface, node string) error {
	if err := CordonNode(ctx, client, node); err != nil {
		return err
	}
	c := exec.Command("kubectl", "drain", node,
		"--ignore-daemonsets",
		"--delete-emptydir-data",
		"--force",
		"--timeout=120s",
		"--grace-period=30",
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func FetchKubeconfig(server, user, key, outPath string) error {
	c := exec.Command("ssh",
		"-i", key,
		"-o", "StrictHostKeyChecking=no",
		user+"@"+server,
		"sudo cat /etc/rancher/k3s/k3s.yaml",
	)
	raw, err := c.Output()
	if err != nil {
		return fmt.Errorf("SSH fetch failed: %w", err)
	}

	content := strings.ReplaceAll(
		string(raw),
		"https://127.0.0.1:6443",
		"https://"+server+":6443",
	)

	if err := os.MkdirAll(dirOf(outPath), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(content), 0600); err != nil {
		return err
	}

	fmt.Println("Saved to " + outPath)
	return nil
}

func dirOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}
