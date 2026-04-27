# rpicli

Go + Cobra CLI for managing the rpi-cluster monorepo — provisioning, platform operations, and cluster lifecycle from a single binary.

## Build

```bash
./scripts/cli-build.sh build        # ./bin/rpicli (current platform)
./scripts/cli-build.sh build --all  # ./bin/rpicli-linux-amd64 + ./bin/rpicli-linux-arm64
./scripts/cli-build.sh install      # build + copy to /usr/local/bin/rpicli
```

Run all commands from the **repo root** so the default config path (`./config/cluster-config.toml`) resolves correctly.

---

## Configuration

rpicli reads two files from `config/`:

| File | Required | Description |
| ---- | -------- | ----------- |
| `cluster-config.toml` | ✅ | Nodes, network, k3s settings, platform versions, DNS |
| `secrets.toml` | Only for cert-manager apply | Sensitive values (Cloudflare API token) |

Override the config path with `--config <path>`.

See [`config/`](../config/) for the files and [`config/secrets.toml.example`](../config/secrets.toml.example) for the secrets template.

---

## Commands

### Global flags

```
--config <path>       Config file (default: ./config/cluster-config.toml)
--kubeconfig <path>   Kubeconfig (default: ~/.kube/rpi-cluster.yaml)
```

---

### `setup` — Ansible provisioning

Wraps `ansible-playbook` using settings from `[setup]` in `cluster-config.toml`.

| Command | Description |
| ------- | ----------- |
| `setup deps` | Install Ansible Galaxy collections (`community.general`, `ansible.posix`) |
| `setup ping` | Pre-flight: connectivity, sudo, NVMe presence |
| `setup deploy` | Full cluster bootstrap (`site.yml`) |
| `setup check` | Dry-run deploy (`--check --diff`) |
| `setup reset` | Tear down k3s — **destructive**, requires confirmation |
| `setup lint` | Run yamllint + ansible-lint on `cluster-setup/` |

**Common flags** (ping, deploy, check, reset):

```
-i, --inventory <name>         Inventory under inventories/ (default: from config)
-l, --limit <pattern>          Restrict to matching hosts
-t, --tags <tags>              Run only tasks with these tags
-e, --extra-var <key=value>    Set extra variable (repeatable)
```

Examples:

```bash
./bin/rpicli setup ping
./bin/rpicli setup deploy
./bin/rpicli setup deploy -l rpi-0 -t common
./bin/rpicli setup check -l rpi-0
```

---

### `platform` — Kubernetes infrastructure

Manages the platform stack. Uses `[platform]` from `cluster-config.toml` and reads `~/.kube/rpi-cluster.yaml` by default.

#### Lifecycle

| Command | Description |
| ------- | ----------- |
| `platform kubeconfig --server <ip>` | Fetch kubeconfig from a control-plane node; SSH user/key from config |
| `platform apply <component\|all>` | Apply one or all components in dependency order |
| `platform diff <component\|all>` | Dry-run diff against live cluster |
| `platform delete <component>` | Remove a component — requires confirmation |
| `platform restart <component>` | Rollout restart a component's workloads |
| `platform lint` | Run yamllint on `cluster-platform/` manifests |

Known components: `kube-vip` · `metallb` · `ingress-nginx` · `longhorn` · `cert-manager`

Apply order: `kube-vip → metallb → ingress-nginx → longhorn → cert-manager`

#### Observability

| Command | Description |
| ------- | ----------- |
| `platform status [component]` | Pod/service status per component (default: all) |
| `platform logs <component\|pod>` | Stream logs; component name auto-resolves to namespace + selector |
| `platform events [-n ns\|-A]` | Warning events sorted by timestamp |
| `platform describe <type> [name] [-n ns]` | `kubectl describe` wrapper |
| `platform exec <component\|pod> [-- cmd]` | Exec into a pod (default: `sh`) |

**logs flags:**

```
--tail <n>          Lines to show (default: 100)
-f, --follow        Follow log output
-c, --container     Specific container name
-n, --namespace     Required when specifying a pod name directly
```

Examples:

```bash
./bin/rpicli platform kubeconfig --server 192.168.50.20
./bin/rpicli platform apply all
./bin/rpicli platform status longhorn
./bin/rpicli platform logs longhorn --tail 50 -f
./bin/rpicli platform logs my-pod -n default
./bin/rpicli platform exec longhorn -- sh
./bin/rpicli platform events -n cert-manager
./bin/rpicli platform diff kube-vip
./bin/rpicli platform delete metallb
```

---

### `cluster` — Cross-cutting operations

| Command | Description |
| ------- | ----------- |
| `cluster shutdown` | Gracefully drain and power off all nodes |

The shutdown sequence:
1. Cordon all nodes
2. Drain agents in parallel
3. Drain joining servers serially (etcd quorum)
4. Drain init server last
5. Uncordon all (so nodes come back schedulable on next boot)
6. SSH `shutdown -h now` — agents → joining servers → init server

SSH user and key are read from `[ssh]` in `cluster-config.toml`.

```bash
./bin/rpicli cluster shutdown
```

---

## Project structure

```
cluster-cli/
├── main.go
├── go.mod
├── cmd/
│   ├── root.go        # global flags, config loading, command registration
│   ├── setup.go       # setup subcommands
│   ├── platform.go    # platform subcommands
│   └── cluster.go     # cluster subcommands
└── internal/
    ├── ansible/       # go-ansible/v2 wrapper (Playbook builder)
    ├── component/     # component registry (name → namespace + selector + workloads)
    ├── config/        # TOML loader for cluster-config.toml and secrets.toml
    ├── kube/          # client-go operations (status, logs, events, cordon, drain)
    └── printer/       # coloured terminal output
```

## Dependencies

| Package | Purpose |
| ------- | ------- |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/apenella/go-ansible/v2` | Ansible playbook execution |
| `k8s.io/client-go` | Native Kubernetes API access |
| `github.com/BurntSushi/toml` | Config file parsing |
| `github.com/fatih/color` | Coloured terminal output |

## Testing

```bash
./scripts/cli-build.sh test    # go test -race with coverage
./scripts/cli-build.sh lint    # go vet
```

Tests use `k8s.io/client-go/kubernetes/fake` — no live cluster required.
