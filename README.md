# rpi-cluster

Monorepo for a 6-node Raspberry Pi Kubernetes cluster — from bare-metal provisioning to application deployment.

## Structure

```
rpi-cluster/
├── cluster-setup/    # Ansible automation — OS hardening, storage, k3s bootstrap
├── cluster-platform/ # GitOps manifests — platform infrastructure
├── cluster-cli/      # Go CLI — rpicli (source)
├── config/           # Cluster-wide configuration (cluster-config.toml, secrets.toml)
├── bin/              # Compiled rpicli binary (gitignored)
└── scripts/          # Build scripts (cli-build.sh, act.sh)
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

GitOps-managed Kubernetes platform layer — applied with `./bin/rpicli platform` after the cluster is bootstrapped.

- kube-vip — HA API endpoint (VIP `192.168.50.30`)
- MetalLB — LoadBalancer IPs on LAN (`192.168.50.40–60`)
- ingress-nginx — HTTP/S routing (`192.168.50.40`)
- Longhorn — replicated block storage on NVMe (`longhorn.kantharos.srv`)
- cert-manager — automatic TLS via self-signed and Let's Encrypt DNS-01

See [`cluster-platform/README.md`](./cluster-platform/README.md) for full documentation.

### [`cluster-cli/`](./cluster-cli/)

Go + Cobra CLI (`rpicli`) — the primary management tool for all cluster operations.

```bash
./scripts/cli-build.sh build     # compile to ./bin/rpicli
./bin/rpicli setup deploy        # Ansible bootstrap
./bin/rpicli platform apply all  # deploy platform stack
./bin/rpicli cluster shutdown    # graceful shutdown
```

### [`config/`](./config/)

Single source of truth for all environment-specific values.

| File | Purpose | Committed |
| ---- | ------- | --------- |
| `cluster-config.toml` | Nodes, network, k3s, storage, platform versions, DNS | ✅ |
| `secrets.toml` | Sensitive values (Cloudflare API token, etc.) | ❌ gitignored |
| `secrets.toml.example` | Template showing secrets structure | ✅ |

### [`scripts/`](./scripts/)

| Script | Description |
| ------ | ----------- |
| `cli-build.sh build [--all]` | Build `./bin/rpicli` (current platform or all targets) |
| `cli-build.sh test` | Run unit tests with coverage |
| `cli-build.sh install` | Build and install to `/usr/local/bin/rpicli` |
| `act.sh lint setup\|platform\|all` | Run CI workflows locally via act |

## CI

| Workflow | File | Triggers on | Jobs |
| -------- | ---- | ----------- | ---- |
| Cluster Setup — Lint | `cluster-setup-lint.yml` | `cluster-setup/**` | YAML Lint, Ansible Lint, Syntax Check |
| Cluster Platform — Lint | `cluster-platform-lint.yml` | `cluster-platform/**` | Manifests Lint |
| Cluster CLI | `cluster-cli.yml` | `cluster-cli/**` | Test, Build (amd64 + arm64) |
