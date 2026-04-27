package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"rpicli/internal/kube"
	"rpicli/internal/printer"

	"github.com/spf13/cobra"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Cross-cutting cluster operations",
}

func init() {
	clusterCmd.AddCommand(clusterShutdownCmd)
}

var (
	sshUser = "rpi"
	sshKey  = ""

	agents         = []string{"rpi-3", "rpi-4", "rpi-5"}
	agentIPs       = []string{"192.168.50.23", "192.168.50.24", "192.168.50.25"}
	joiningServers = []string{"rpi-2", "rpi-1"}
	joiningIPs     = []string{"192.168.50.22", "192.168.50.21"}
	initServer     = "rpi-0"
	initIP         = "192.168.50.20"
)

var clusterShutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Gracefully drain and power off all nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		sshUser, _ = cmd.Flags().GetString("ssh-user")
		sshKey, _ = cmd.Flags().GetString("ssh-key")

		client, err := newKubeClient()
		if err != nil {
			return err
		}
		ctx := context.Background()

		printer.Header("Current cluster state")
		if err := kube.PrintNodes(ctx, client); err != nil {
			return err
		}
		fmt.Println()

		printer.Warn("This will GRACEFULLY SHUT DOWN all 6 Raspberry Pi nodes.")
		printer.Warn("All workloads will be evicted. The cluster will be offline.")
		fmt.Println()
		if !printer.Confirm("Type 'shutdown' to confirm") {
			printer.Info("Aborted.")
			return nil
		}

		allNodes := append(append(agents, joiningServers...), initServer)

		printer.Header("Step 1/4 — Cordon all nodes")
		printer.Info("Cordoning prevents the scheduler from placing new pods on any node")
		printer.Info("while we drain. Existing pods keep running until explicitly evicted.")
		fmt.Println()
		for _, node := range allNodes {
			if err := kube.CordonNode(ctx, client, node); err != nil {
				printer.Warn("Could not cordon " + node + ": " + err.Error())
			} else {
				printer.Info("  Cordoned " + node)
			}
		}

		uncordonAll := func() {
			printer.Warn("Uncordoning all nodes — restoring schedulability...")
			for _, node := range allNodes {
				_ = kube.UncordonNode(ctx, client, node)
				printer.Info("  Uncordoned " + node)
			}
		}

		printer.Header("Step 2/4 — Drain agents (parallel)")
		printer.Info("Draining evicts all non-daemonset pods from the workers.")
		printer.Info("Pods with persistent volumes (Longhorn) are gracefully unmounted first.")
		fmt.Println()
		var wg sync.WaitGroup
		for _, node := range agents {
			wg.Add(1)
			go func(n string) {
				defer wg.Done()
				printer.Info("  Draining " + n + "...")
				if err := kube.DrainNode(ctx, client, n); err != nil {
					printer.Warn("  " + n + " drain error: " + err.Error())
				} else {
					printer.Success("  " + n + " drained.")
				}
			}(node)
		}
		wg.Wait()

		printer.Header("Step 3/4 — Drain control-plane nodes (serial — etcd quorum)")
		printer.Info("Servers are drained one at a time to maintain etcd quorum (2/3).")
		printer.Info("rpi-0 (init server / likely etcd leader) goes last.")
		fmt.Println()
		for _, node := range append(joiningServers, initServer) {
			printer.Info("  Draining " + node + "...")
			if err := kube.DrainNode(ctx, client, node); err != nil {
				printer.Warn("  " + node + " drain error: " + err.Error())
			} else {
				printer.Success("  " + node + " drained.")
			}
		}

		printer.Header("Step 4/4 — Power off nodes")
		printer.Info("Uncordoning before shutdown so nodes come back schedulable")
		printer.Info("if they reboot instead of halting, or if shutdown fails.")
		fmt.Println()
		uncordonAll()
		fmt.Println()

		sshShutdown := func(label, ip string) {
			c := exec.Command("ssh",
				"-i", sshKey,
				"-o", "StrictHostKeyChecking=no",
				"-o", "ConnectTimeout=10",
				sshUser+"@"+ip,
				"sudo shutdown -h now",
			)
			if err := c.Run(); err != nil {
				printer.Warn("  " + label + " (" + ip + ") — unreachable")
			} else {
				printer.Info("  " + label + " (" + ip + ") — shutdown sent")
			}
		}

		printer.Info("Agents (simultaneous)...")
		var wg2 sync.WaitGroup
		for i, node := range agents {
			wg2.Add(1)
			go func(n, ip string) { defer wg2.Done(); sshShutdown(n, ip) }(node, agentIPs[i])
		}
		wg2.Wait()
		time.Sleep(5 * time.Second)

		printer.Info("Joining servers (simultaneous)...")
		var wg3 sync.WaitGroup
		for i, node := range joiningServers {
			wg3.Add(1)
			go func(n, ip string) { defer wg3.Done(); sshShutdown(n, ip) }(node, joiningIPs[i])
		}
		wg3.Wait()
		time.Sleep(5 * time.Second)

		printer.Info("Init server (rpi-0 — last)...")
		sshShutdown(initServer, initIP)

		fmt.Println()
		printer.Success("All nodes shutting down. Cluster is offline.")
		return nil
	},
}

func init() {
	home, _ := homeDir()
	clusterShutdownCmd.Flags().String("ssh-user", "rpi", "SSH user for node access")
	clusterShutdownCmd.Flags().String("ssh-key", home+"/.ssh/id_rpi", "SSH private key path")
}

func homeDir() (string, error) {
	h, err := exec.Command("sh", "-c", "echo $HOME").Output()
	if err != nil {
		return "", err
	}
	if len(h) > 0 && h[len(h)-1] == '\n' {
		h = h[:len(h)-1]
	}
	return string(h), nil
}
