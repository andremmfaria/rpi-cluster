# rpi-cluster

Monorepo for a 6-node Raspberry Pi Kubernetes cluster — from bare-metal provisioning to application deployment.

## Structure

```
rpi-cluster/
├── cluster-setup/        # Ansible automation — OS hardening, storage, k3s bootstrap
└── cluster-platform/     # GitOps manifests — platform infrastructure (kube-vip, MetalLB, ingress)
```

## Modules

### [`cluster-setup/`](./cluster-setup/README.md)

Ansible playbooks and roles that take freshly imaged Raspberry Pi nodes to a fully operational k3s HA cluster.

- OS hardening, cgroup config, WiFi disable, SSH key deployment
- USB-attached NVMe provisioning (partition, format, mount)
- k3s embedded-etcd HA control plane (3 servers + 3 agents)
- Post-install node labelling and health verification

See [`cluster-setup/README.md`](./cluster-setup/README.md) for full documentation.

### [`cluster-platform/`](./cluster-platform/README.md)

GitOps-managed Kubernetes platform layer — applied with `kubectl` after the cluster is bootstrapped.

- kube-vip — HA API endpoint (VIP `192.168.50.30`)
- MetalLB — LoadBalancer IPs on LAN (`192.168.50.40–60`)
- ingress-nginx — HTTP/S routing (`192.168.50.40`)

See [`cluster-platform/README.md`](./cluster-platform/README.md) for full documentation.

## CI

| Workflow | File | Triggers on | Jobs |
| -------- | ---- | ----------- | ---- |
| Cluster Setup — Lint | `cluster-setup-lint.yml` | `cluster-setup/**` | YAML Lint, Ansible Lint, Syntax Check |
| Cluster Platform — Lint | `cluster-platform-lint.yml` | `cluster-platform/**` | Manifests Lint |
