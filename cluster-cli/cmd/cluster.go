package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"rpicli/internal/config"
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

var clusterShutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Gracefully drain and power off all nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		sshUser := cfg.SSH.User
		sshKey := cfg.SSH.Key

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

		printer.Warn("This will GRACEFULLY SHUT DOWN all cluster nodes.")
		printer.Warn("All workloads will be evicted. The cluster will be offline.")
		fmt.Println()
		if !printer.Confirm("Type 'shutdown' to confirm") {
			printer.Info("Aborted.")
			return nil
		}

		agents := cfg.Nodes.Agents
		joining := cfg.JoiningServers()
		init := cfg.InitServer()

		allNodes := make([]string, 0, len(agents)+len(joining)+1)
		for _, n := range agents {
			allNodes = append(allNodes, n.Name)
		}
		for _, n := range joining {
			allNodes = append(allNodes, n.Name)
		}
		allNodes = append(allNodes, init.Name)

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
			go func(n config.Node) {
				defer wg.Done()
				printer.Info("  Draining " + n.Name + "...")
				if err := kube.DrainNode(ctx, client, n.Name); err != nil {
					printer.Warn("  " + n.Name + " drain error: " + err.Error())
				} else {
					printer.Success("  " + n.Name + " drained.")
				}
			}(node)
		}
		wg.Wait()

		printer.Header("Step 3/4 — Drain control-plane nodes (serial — etcd quorum)")
		printer.Info("Servers are drained one at a time to maintain etcd quorum (2/3).")
		printer.Info(fmt.Sprintf("%s (init server / likely etcd leader) goes last.", init.Name))
		fmt.Println()
		for _, n := range joining {
			printer.Info("  Draining " + n.Name + "...")
			if err := kube.DrainNode(ctx, client, n.Name); err != nil {
				printer.Warn("  " + n.Name + " drain error: " + err.Error())
			} else {
				printer.Success("  " + n.Name + " drained.")
			}
		}
		printer.Info("  Draining " + init.Name + "...")
		if err := kube.DrainNode(ctx, client, init.Name); err != nil {
			printer.Warn("  " + init.Name + " drain error: " + err.Error())
		} else {
			printer.Success("  " + init.Name + " drained.")
		}

		printer.Header("Step 4/4 — Power off nodes")
		printer.Info("Uncordoning before shutdown so nodes come back schedulable")
		printer.Info("if they reboot instead of halting, or if shutdown fails.")
		fmt.Println()
		uncordonAll()
		fmt.Println()

		sshShutdown := func(n config.Node) {
			c := exec.Command("ssh",
				"-i", sshKey,
				"-o", "StrictHostKeyChecking=no",
				"-o", "ConnectTimeout=10",
				sshUser+"@"+n.IP,
				"sudo shutdown -h now",
			)
			if err := c.Run(); err != nil {
				printer.Warn("  " + n.Name + " (" + n.IP + ") — unreachable")
			} else {
				printer.Info("  " + n.Name + " (" + n.IP + ") — shutdown sent")
			}
		}

		printer.Info("Agents (simultaneous)...")
		var wg2 sync.WaitGroup
		for _, n := range agents {
			wg2.Add(1)
			go func(node config.Node) { defer wg2.Done(); sshShutdown(node) }(n)
		}
		wg2.Wait()
		time.Sleep(5 * time.Second)

		printer.Info("Joining servers (simultaneous)...")
		var wg3 sync.WaitGroup
		for _, n := range joining {
			wg3.Add(1)
			go func(node config.Node) { defer wg3.Done(); sshShutdown(node) }(n)
		}
		wg3.Wait()
		time.Sleep(5 * time.Second)

		printer.Info(fmt.Sprintf("Init server (%s — last)...", init.Name))
		sshShutdown(init)

		fmt.Println()
		printer.Success("All nodes shutting down. Cluster is offline.")
		return nil
	},
}
