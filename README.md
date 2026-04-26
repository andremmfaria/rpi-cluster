# rpi-cluster

Monorepo for a 6-node Raspberry Pi Kubernetes cluster — from bare-metal provisioning to application deployment.

## Structure

```
rpi-cluster/
├── cluster-setup/        # Ansible automation — OS hardening, storage, k3s bootstrap
└── cluster-platform/     # Platform configuration — cluster-level services and apps (coming soon)
```

## Modules

### [`cluster-setup/`](./cluster-setup/README.md)

Ansible playbooks and roles that take freshly imaged Raspberry Pi nodes to a fully operational k3s HA cluster.

- OS hardening, cgroup config, WiFi disable, SSH key deployment
- USB-attached NVMe provisioning (partition, format, mount)
- k3s embedded-etcd HA control plane (3 servers + 3 agents)
- Post-install node labelling and health verification

See [`cluster-setup/README.md`](./cluster-setup/README.md) for full documentation.

### `cluster-platform/` _(coming soon)_

Cluster-level platform services deployed on top of the k3s cluster — ingress, storage classes, monitoring, and application workloads.

## CI

| Workflow | Triggers on | Jobs |
| -------- | ----------- | ---- |
| Lint     | `cluster-setup/**` | YAML Lint, Ansible Lint, Syntax Check |
