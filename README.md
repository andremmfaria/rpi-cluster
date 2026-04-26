# rpi-cluster

Ansible automation for a bare-metal Raspberry Pi Kubernetes cluster running [k3s](https://k3s.io/).

---

## Table of Contents

- [Overview](#overview)
- [Cluster Architecture](#cluster-architecture)
- [Hardware Requirements](#hardware-requirements)
- [Network Layout](#network-layout)
- [Prerequisites](#prerequisites)
- [Repository Structure](#repository-structure)
- [Configuration Reference](#configuration-reference)
  - [Inventory](#inventory)
  - [Group Variables](#group-variables)
- [Roles](#roles)
  - [common](#role-common)
  - [storage](#role-storage)
  - [k3s_server](#role-k3s_server)
  - [k3s_agent](#role-k3s_agent)
  - [k3s_post](#role-k3s_post)
- [Playbooks](#playbooks)
  - [00-ping.yml — Pre-flight validation](#00-pingyml--pre-flight-validation)
  - [site.yml — Full bootstrap](#siteyml--full-bootstrap)
  - [99-reset.yml — Cluster teardown](#99-resetyml--cluster-teardown)
- [Bootstrap Workflow](#bootstrap-workflow)
- [Security Considerations](#security-considerations)
- [Variable Precedence](#variable-precedence)
- [CI](#ci)
- [Troubleshooting](#troubleshooting)

---

## Overview

This project provisions a 6-node Raspberry Pi cluster:

- **3 control-plane nodes** (`rpi-0`, `rpi-1`, `rpi-2`) running k3s in embedded-etcd HA mode.
- **3 worker/agent nodes** (`rpi-3`, `rpi-4`, `rpi-5`) running the k3s agent.

All nodes boot from SD card but store Kubernetes state on NVMe SSDs connected over USB3. Traefik, ServiceLB, and local-storage are disabled in favour of externally managed replacements.

---

## Cluster Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    192.168.50.0/24                      │
│                                                         │
│  rpi-0 (.20) ──┐                                        │
│  rpi-1 (.21) ──┼── k3s HA (embedded etcd, port 6443)    │
│  rpi-2 (.22) ──┘                                        │
│                                                         │
│  rpi-3 (.23) ──┐                                        │
│  rpi-4 (.24) ──┼── k3s agents                           │
│  rpi-5 (.25) ──┘                                        │
│                                                         │
│  VIP: 192.168.50.30  (TLS SAN, external load-balancer)  │
└─────────────────────────────────────────────────────────┘

Pod CIDR:     10.42.0.0/16
Service CIDR: 10.43.0.0/16
```

`rpi-0` is the bootstrap leader. Remaining servers join via `--server https://rpi-0:6443`. Agents join via the same endpoint (or the VIP once a load-balancer is in place).

---

## Hardware Requirements

| Component | Minimum                                                                     |
| --------- | --------------------------------------------------------------------------- |
| SBC       | Raspberry Pi 4 or 5 (aarch64)                                               |
| RAM       | 4 GB per node                                                               |
| SD card   | 16 GB (OS only)                                                             |
| NVMe SSD  | >200 GB per node, connected via USB3 NVMe enclosure — exposed as `/dev/sda` |
| Network   | Gigabit Ethernet                                                            |

> WiFi is disabled on all nodes at boot (both `dtoverlay` and `rfkill`). All traffic must route over Ethernet.

---

## Network Layout

| Host    | IP              | Role                            |
| ------- | --------------- | ------------------------------- |
| `rpi-0` | `192.168.50.20` | k3s server (bootstrap leader)   |
| `rpi-1` | `192.168.50.21` | k3s server                      |
| `rpi-2` | `192.168.50.22` | k3s server                      |
| `rpi-3` | `192.168.50.23` | k3s agent                       |
| `rpi-4` | `192.168.50.24` | k3s agent                       |
| `rpi-5` | `192.168.50.25` | k3s agent                       |
| VIP     | `192.168.50.30` | API server virtual IP (TLS SAN) |

---

## Prerequisites

### Control machine

- Ansible ≥ 2.14
- Python 3 with `netaddr` library (`pip install netaddr`)
- SSH key at `~/.ssh/id_ed25519` (public key deployed to all nodes)

### Nodes

- Raspberry Pi OS (64-bit, Lite)
- SSH enabled, `rpi` user with sudo access
- NVMe disk visible as `/dev/sda`

### Ansible collections

Install before first run:

```bash
ansible-galaxy collection install community.general ansible.posix
```

Both collections are required:

| Collection          | Used for                          |
| ------------------- | --------------------------------- |
| `community.general` | `parted`, `modprobe` modules      |
| `ansible.posix`     | `authorized_key`, `mount` modules |

---

## Repository Structure

```
rpi-cluster/
├── inventories/
│   └── homelab/
│       ├── hosts.yml                  # Node IPs and group membership
│       └── group_vars/
│           └── all.yml                # Cluster-wide variables (edit before running)
├── playbooks/
│   ├── 00-ping.yml                    # Pre-flight hardware & OS validation
│   └── 99-reset.yml                   # Full cluster teardown
├── roles/
│   ├── common/                        # OS hardening, kernel modules, SSH
│   │   ├── defaults/main.yml
│   │   ├── handlers/main.yml
│   │   └── tasks/main.yml
│   ├── storage/                       # NVMe partitioning, formatting, mounting
│   │   ├── defaults/main.yml
│   │   └── tasks/main.yml
│   ├── k3s_server/                    # k3s server installation
│   │   ├── defaults/main.yml
│   │   └── tasks/main.yml
│   ├── k3s_agent/                     # k3s agent installation
│   │   ├── defaults/main.yml
│   │   └── tasks/main.yml
│   └── k3s_post/                      # Post-install: wait for nodes, apply labels
│       ├── defaults/main.yml
│       └── tasks/main.yml
├── scripts/
│   ├── run.sh                         # Ansible wrapper (deps/ping/deploy/reset/check/lint)
│   └── act.sh                         # Local CI runner via act (lint/all)
└── site.yml                           # Master playbook
```

---

## Configuration Reference

### Inventory

**`inventories/homelab/hosts.yml`**

Defines the two Ansible groups used throughout all playbooks:

- `k3s_servers` — nodes that run the k3s control plane with embedded etcd.
- `k3s_agents` — nodes that run the k3s agent (worker-only, no etcd).
- `rpi_cluster` — parent group containing both; used for plays targeting all nodes.

Each host declares two variables:

| Variable       | Purpose                                                   |
| -------------- | --------------------------------------------------------- |
| `ansible_host` | IP address Ansible connects to                            |
| `node_ip`      | IP advertised to k3s (`--node-ip`, `--advertise-address`) |

Both are set to the same value for a flat-network setup. Separate them only if Ansible control traffic and k3s data-plane traffic need to flow over different interfaces.

---

### Group Variables

**`inventories/homelab/group_vars/all.yml`**

All mandatory customisation lives here. Edit this file before running any playbook.

#### Ansible connection

| Variable                  | Default                       | Description                                                          |
| ------------------------- | ----------------------------- | -------------------------------------------------------------------- |
| `ansible_user`            | `rpi`                         | SSH user on all nodes                                                |
| `ansible_password`        | `rpi_passwd`                  | SSH password (initial bootstrap only; disabled after key deployment) |
| `ansible_become_password` | `rpi_passwd`                  | sudo password                                                        |
| `ansible_ssh_common_args` | `-o StrictHostKeyChecking=no` | Disables host-key checking; remove once keys are known-good          |

> **Security**: `ansible_password` and `ansible_become_password` are cleartext here as a bootstrap convenience. Move them to an Ansible Vault file (`ansible-vault create group_vars/all/vault.yml`) before committing or sharing this repo.

#### k3s

| Variable      | Default                               | Description                                 |
| ------------- | ------------------------------------- | ------------------------------------------- |
| `k3s_version` | `v1.35.3+k3s1`                        | k3s release to install (pin this)           |
| `k3s_token`   | `CHANGE_THIS_TO_A_LONG_RANDOM_SECRET` | Shared cluster secret — **must be changed** |
| `k3s_api_vip` | `192.168.50.30`                       | Added as TLS SAN; set to `""` to skip       |

> Generate a strong token: `openssl rand -hex 32`

#### Networking

| Variable       | Default        | Description             |
| -------------- | -------------- | ----------------------- |
| `cluster_cidr` | `10.42.0.0/16` | Pod IP range            |
| `service_cidr` | `10.43.0.0/16` | ClusterIP service range |

#### Storage

| Variable          | Default                                               | Description                              |
| ----------------- | ----------------------------------------------------- | ---------------------------------------- |
| `nvme_disk`       | `/dev/sda`                                            | Raw NVMe block device                    |
| `nvme_device`     | `/dev/sda1`                                           | Partition that will be formatted/mounted |
| `nvme_mount`      | `/mnt/nvme`                                           | Mount point                              |
| `nvme_fstype`     | `ext4`                                                | Filesystem type                          |
| `nvme_mount_opts` | `defaults,noatime,nofail,x-systemd.device-timeout=10` | fstab options                            |
| `k3s_data_dir`    | `/mnt/nvme/k3s`                                       | k3s `--data-dir`; must be on NVMe        |

#### Cluster service account

A non-login system user/group (`k3s`/`k3s`) owns k3s runtime directories. UIDs/GIDs are pinned for consistency across nodes.

| Variable                | Default | Description       |
| ----------------------- | ------- | ----------------- |
| `cluster_service_user`  | `k3s`   | System user name  |
| `cluster_service_group` | `k3s`   | System group name |
| `cluster_service_uid`   | `1050`  | UID (pinned)      |
| `cluster_service_gid`   | `1050`  | GID (pinned)      |

#### SSH hardening

| Variable                    | Default                 | Description                                  |
| --------------------------- | ----------------------- | -------------------------------------------- |
| `ssh_admin_user`            | `rpi`                   | User that retains SSH access after hardening |
| `ssh_admin_pubkey_path`     | `~/.ssh/id_ed25519.pub` | Public key deployed to all nodes             |
| `disable_ssh_password_auth` | `true`                  | Disables password auth after key deployment  |

---

## Roles

### Role: `common`

**Applied to**: all nodes (`rpi_cluster`)

**Defaults**: `roles/common/defaults/main.yml`

Sets up every node identically before any Kubernetes component is installed.

#### Task summary

| Task                       | Detail                                                                                                                                                              |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Set hostname               | Sets `inventory_hostname` as the system hostname and adds a `127.0.1.1` entry to `/etc/hosts`                                                                       |
| Update apt cache           | Refreshes with a 1-hour validity window                                                                                                                             |
| Install packages           | Installs the list in `required_packages` (see defaults)                                                                                                             |
| Create service group/user  | Creates the `k3s` system account (UID 1050, no login, no home)                                                                                                      |
| Create runtime directories | `/etc/rancher`, `/var/lib/rancher`, `/var/lib/kubelet`, `/var/lib/cni`, `/run/k3s` — owned by the service user                                                      |
| tmpfiles.d                 | Writes `/etc/tmpfiles.d/k3s-cluster-user.conf` so systemd recreates the runtime dirs on boot                                                                        |
| Sudo for admin user        | Installs a `NOPASSWD` sudoers file for `ssh_admin_user`                                                                                                             |
| Deploy SSH key             | Uses `ansible.posix.authorized_key` with `exclusive: true` — **this removes all other authorized keys** from the admin user                                         |
| Harden sshd                | Disables `PasswordAuthentication`, `KbdInteractiveAuthentication`, and `ChallengeResponseAuthentication` when `disable_ssh_password_auth` is true                   |
| Block service user SSH     | Adds a `DenyUsers` block for the `k3s` system user                                                                                                                  |
| Kernel modules             | Loads `br_netfilter` and `overlay`; persists via `/etc/modules-load.d/k3s.conf`                                                                                     |
| sysctl                     | Writes `/etc/sysctl.d/99-k3s.conf` and applies via `sysctl --system`                                                                                                |
| Boot cmdline               | Appends `cgroup_enable=cpuset cgroup_enable=memory cgroup_memory=1` to `/boot/firmware/cmdline.txt` — deduplicated with `unique` filter; triggers reboot if changed |
| Disable WiFi               | `dtoverlay=disable-wifi` in `/boot/firmware/config.txt`; blacklists `brcmfmac`/`brcmutil`; calls `rfkill block wifi`                                                |

#### Handlers

| Handler        | Trigger                            | Action                                      |
| -------------- | ---------------------------------- | ------------------------------------------- |
| `Reboot node`  | cmdline.txt or WiFi config changed | `ansible.builtin.reboot` with 300 s timeout |
| `Restart ssh`  | any sshd_config change             | `systemctl restart ssh`                     |

> **Note**: The reboot handler flushes at end of the `common` role play, not inline. If cmdline.txt is changed, all subsequent tasks in the same play run on the rebooted node.

#### Default packages (`required_packages`)

`ca-certificates`, `curl`, `e2fsprogs`, `htop`, `iotop`, `jq`, `nfs-common`, `nvme-cli`, `openssh-server`, `parted`, `python3`, `python3-apt`, `smartmontools`, `sudo`, `sysstat`, `xfsprogs`

---

### Role: `storage`

**Applied to**: all nodes (`rpi_cluster`)

**Defaults**: `roles/storage/defaults/main.yml`

Provisions the NVMe SSD on each node and creates the k3s data directory.

#### Task flow

```
stat /dev/sda1 (nvme_device)
    │
    ├─ exists ──────────────────────────────────────────────────────┐
    │                                                               │
    └─ missing → stat /dev/sda (nvme_disk)                         │
                     │                                             │
                     ├─ missing → FAIL                             │
                     │                                             │
                     └─ exists → parted: partition 1 (GPT, 1MiB–100%)
                                 wait_for: /dev/sda1 appears        │
                                                                    ↓
                              blkid -s TYPE /dev/sda1 ─────────────┘
                                  │
                                  ├─ rc=0 (filesystem found) → skip format
                                  └─ rc≠0 (no filesystem) → mkfs.ext4

mkdir /mnt/nvme
blkid -s UUID /dev/sda1
mount (ansible.posix.mount → fstab entry + mount)
mountpoint -q /mnt/nvme (fails play if not mounted)
mkdir /mnt/nvme/k3s (owned by cluster_service_user:cluster_service_group)
```

#### Key behaviours

- Partitioning is **only triggered** if `/dev/sda1` does not already exist. Safe to re-run.
- Filesystem creation is guarded by `blkid` exit code — will not reformat a mounted volume.
- Mount is persisted in `/etc/fstab` by UUID (not device path) for resilience to device renaming.
- The `nofail` mount option prevents boot failures if the NVMe is unplugged.
- The `x-systemd.device-timeout=10` caps udev wait time to 10 seconds.

---

### Role: `k3s_server`

**Applied to**: `k3s_servers` group (serial: 1 for joining servers)

**Defaults**: `roles/k3s_server/defaults/main.yml`

Installs the k3s server binary and starts the systemd service.

#### Installation modes

Controlled by `k3s_cluster_init` (boolean, passed as a `vars:` override from `site.yml`):

| `k3s_cluster_init` | Play target      | Flag passed to k3s                                       |
| ------------------ | ---------------- | -------------------------------------------------------- |
| `true`             | `rpi-0` only     | `--cluster-init` (initialises etcd)                      |
| `false`            | `rpi-1`, `rpi-2` | `--server https://<rpi-0-ip>:6443` (joins existing etcd) |

The first server **must** complete and have port 6443 open before joining servers run. `site.yml` enforces this by running the bootstrap play first (no `serial`), then the join play with `serial: 1`.

#### Server arguments (`k3s_server_extra_args`)

| Argument                      | Reason                                            |
| ----------------------------- | ------------------------------------------------- |
| `--disable=traefik`           | External ingress controller expected              |
| `--disable=servicelb`         | External load-balancer (e.g., MetalLB) expected   |
| `--disable=local-storage`     | NVMe-backed storage class expected                |
| `--write-kubeconfig-mode=644` | Allows non-root kubectl access on the server node |
| `--cluster-cidr`              | Matches `cluster_cidr` from group_vars            |
| `--service-cidr`              | Matches `service_cidr` from group_vars            |
| `--node-ip`                   | Binds k3s to the correct interface IP             |
| `--advertise-address`         | IP advertised to the API server                   |
| `--data-dir`                  | Points to NVMe mount (set via `k3s_data_dir`)     |

If `k3s_api_vip` is non-empty, `--tls-san=<vip>` is appended so the API certificate is valid for external load-balancer traffic. This is computed in a single `set_fact` task at the start of the role using a Jinja2 ternary.

#### Security

- Install shell commands use `no_log: true` to prevent `K3S_TOKEN` from appearing in Ansible output or log files.

#### Post-install check

Waits up to 180 seconds for port 6443 to open on `node_ip` before declaring success.

---

### Role: `k3s_agent`

**Applied to**: `k3s_agents` group (all agents in parallel)

**Defaults**: `roles/k3s_agent/defaults/main.yml`

Installs the k3s agent and registers the node with the cluster.

#### Agent arguments (`k3s_agent_extra_args`)

| Argument     | Reason                               |
| ------------ | ------------------------------------ |
| `--node-ip`  | Binds agent to the correct interface |
| `--data-dir` | Points to NVMe mount                 |

The agent connects to `k3s_server_url`, which defaults to `https://<rpi-0 node_ip>:6443`.

#### Security

- Install shell command uses `no_log: true` — same rationale as `k3s_server`.

---

### Role: `k3s_post`

**Applied to**: `rpi-0` (the bootstrap server)

**Defaults**: `roles/k3s_post/defaults/main.yml`

Runs after all nodes are installed to verify cluster health and apply node labels.

#### Tasks

1. **Wait for all nodes** — polls `kubectl get nodes --no-headers | wc -l` every 10 seconds until the count matches `expected_node_count` (default: total node count in `rpi_cluster`). Up to 40 retries (6 min 40 s).
2. **Label server nodes** — applies `server_node_labels` to every host in `k3s_servers`.
3. **Label agent nodes** — applies `agent_node_labels` to every host in `k3s_agents`.
4. **Print node status** — runs `kubectl get nodes -o wide` and prints output.

#### Default labels

**Server nodes** (`k3s_servers`):

| Label                                   | Value  |
| --------------------------------------- | ------ |
| `node-role.kubernetes.io/control-plane` | `true` |
| `node.kubernetes.io/storage`            | `nvme` |

**Agent nodes** (`k3s_agents`):

| Label                            | Value  |
| -------------------------------- | ------ |
| `node-role.kubernetes.io/worker` | `true` |
| `node.kubernetes.io/storage`     | `nvme` |

Label idempotency: `--overwrite` is passed; `changed_when` inspects kubectl output — the task reports `changed` only when kubectl says `labeled` (value actually set), and `ok` when it says `not labeled` (value unchanged).

---

## Playbooks

### `00-ping.yml` — Pre-flight validation

**Target**: `rpi_cluster` | `gather_facts: false` | `become: true`

Run this before `site.yml` to catch configuration problems early. It performs no destructive actions.

#### Checks performed

| Category     | Check                                | Failure behaviour         |
| ------------ | ------------------------------------ | ------------------------- |
| Connectivity | `ansible.builtin.ping`               | Hard fail                 |
| Privilege    | `whoami` == `root`                   | Hard fail                 |
| Hardware     | `/dev/sda` block device present      | Hard fail                 |
| Hardware     | `/dev/sda` > 200 GB                  | Hard fail                 |
| Hardware     | `/dev/sda1` partition present        | Warn only                 |
| Filesystem   | Filesystem type on `/dev/sda1`       | Print info                |
| Mountpoint   | `/mnt/nvme` exists                   | Warn only                 |
| OS           | Architecture is `aarch64` or `arm64` | Hard fail                 |
| OS           | Kernel version                       | Print info                |
| k3s prereqs  | `cgroup_memory=1` in cmdline.txt     | Warn (fixed by bootstrap) |
| k3s prereqs  | `br_netfilter` loaded                | Warn (fixed by bootstrap) |
| k3s prereqs  | `ip_forward` = 1                     | Warn (fixed by bootstrap) |
| Network      | Default route present                | Hard fail                 |
| Network      | DNS (`getent hosts google.com`)      | Warn only                 |

#### Usage

```bash
ansible-playbook -i inventories/homelab/hosts.yml playbooks/00-ping.yml
```

---

### `site.yml` — Full bootstrap

Runs all five roles in dependency order:

```
Play 1: rpi_cluster      → common, storage          (all nodes, parallel)
Play 2: rpi-0            → k3s_server (init=true)   (bootstrap leader)
Play 3: k3s_servers !rpi-0 → k3s_server (init=false) (serial: 1 — etcd membership constraint)
Play 4: k3s_agents       → k3s_agent               (all agents, parallel)
Play 5: rpi-0            → k3s_post                 (label + verify)
```

`serial: 1` on the joining-servers play is a hard etcd requirement — new members must join one at a time so etcd can commit each membership change and maintain quorum. Agents have no such constraint and install in parallel.

#### Usage

```bash
ansible-playbook -i inventories/homelab/hosts.yml site.yml
```

---

### `99-reset.yml` — Cluster teardown

**Target**: `rpi_cluster` | `become: true`

> **Destructive.** This playbook is intentionally separate from `scripts/run.sh` and must be run explicitly.

Runs the k3s uninstall scripts and wipes the data directory. Does **not** unmount or reformat the NVMe — the filesystem and mount remain intact for re-bootstrapping.

#### Tasks

| Task                     | Server nodes | Agent nodes |
| ------------------------ | ------------ | ----------- |
| `k3s-uninstall.sh`       | ✅           | ❌          |
| `k3s-agent-uninstall.sh` | ❌           | ✅          |
| Remove `k3s_data_dir`    | ✅           | ✅          |
| Recreate `k3s_data_dir`  | ✅           | ✅          |

`failed_when: false` is set on uninstall commands so the playbook succeeds even if k3s was never installed (idempotent teardown).

#### Usage

```bash
ansible-playbook -i inventories/homelab/hosts.yml playbooks/99-reset.yml
```

---

## Bootstrap Workflow

### 1. Prepare group_vars

```yaml
# inventories/homelab/group_vars/all.yml
k3s_token: "<output of: openssl rand -hex 32>"
ansible_password: "<initial rpi user password>"
ansible_become_password: "<initial rpi user password>"
ssh_admin_pubkey_path: "~/.ssh/id_ed25519.pub"
```

### 2. Validate nodes

```bash
ansible-galaxy collection install community.general ansible.posix
ansible-playbook -i inventories/homelab/hosts.yml playbooks/00-ping.yml
```

Review warnings. All hard-fail checks must pass before continuing.

### 3. Bootstrap the cluster

```bash
ansible-playbook -i inventories/homelab/hosts.yml site.yml
```

Or use the helper script (does steps 2 + 3 together):

```bash
./scripts/run.sh deps
./scripts/run.sh ping
./scripts/run.sh deploy
```

Approximate runtime: 10–20 minutes depending on network speed and node responsiveness.

### 4. Verify

From `rpi-0` (or with the kubeconfig copied locally):

```bash
kubectl get nodes -o wide
# All 6 nodes should be Ready
```

### 5. Teardown (when needed)

```bash
ansible-playbook -i inventories/homelab/hosts.yml playbooks/99-reset.yml
```

---

## Security Considerations

### Secrets in group_vars

`all.yml` contains `ansible_password`, `ansible_become_password`, and `k3s_token` in plaintext. This is acceptable for a local homelab but should not be committed to a shared repository.

**Recommended approach**:

```bash
ansible-vault create inventories/homelab/group_vars/vault.yml
# Add to vault.yml:
# vault_ansible_password: "..."
# vault_ansible_become_password: "..."
# vault_k3s_token: "..."

# Reference in all.yml:
# ansible_password: "{{ vault_ansible_password }}"
# ansible_become_password: "{{ vault_ansible_become_password }}"
# k3s_token: "{{ vault_k3s_token }}"
```

Then run playbooks with `--ask-vault-pass` or `--vault-password-file`.

### SSH hardening sequence

The `common` role intentionally:

1. Deploys the SSH public key first.
2. Only then disables password authentication.

This prevents lockout. However, if `ssh_admin_pubkey_path` points to a missing or wrong key, you will lose SSH access. Verify the key before running with `disable_ssh_password_auth: true`.

### `exclusive: true` on authorized_key

The admin key is deployed with `exclusive: true`, which **removes all other keys** from `~/.ssh/authorized_keys` on the target user. This is intentional for security but means any manually added keys will be purged on each run.

### No-log on k3s install

Both `k3s_server` and `k3s_agent` install tasks use `no_log: true` to prevent `K3S_TOKEN` from appearing in Ansible output, callback plugins, or log aggregators.

### StrictHostKeyChecking

`ansible_ssh_common_args: "-o StrictHostKeyChecking=no"` is set for initial bootstrap convenience. After all nodes are known, remove this option and add their host keys to `~/.ssh/known_hosts`.

---

## Variable Precedence

Variables are defined in multiple places. Ansible's precedence (highest wins):

1. `inventories/homelab/group_vars/all.yml` — **primary source of truth**; overrides role defaults.
2. `roles/<role>/defaults/main.yml` — fallback defaults, used only if not set in group_vars.

Role defaults intentionally mirror group_var keys (e.g., `k3s_version`, `k3s_token`, `k3s_data_dir`) so roles are independently testable against a real inventory. When running via `site.yml` with the homelab inventory, group_vars always win.

---

## CI

GitHub Actions runs on every push and pull request to `main`/`master`:

| Job            | What it does                                          |
| -------------- | ----------------------------------------------------- |
| YAML Lint      | `yamllint .` against `.yamllint.yml`                  |
| Ansible Lint   | `ansible-lint` with profile `basic`                   |
| Syntax Check   | `ansible-playbook --syntax-check` on all three plays  |

No molecule/container tests — role logic is validated on physical hardware. Use `./scripts/run.sh check` for a live dry-run against the real inventory before applying.

To run the same checks locally:

```bash
./scripts/run.sh lint          # yamllint + ansible-lint
./scripts/act.sh lint          # full CI workflow via act (requires Docker)
./scripts/act.sh all           # all CI workflows via act
```

---

## Troubleshooting

### Node not joining cluster

- Check port 6443 is reachable from the joining node: `nc -zv 192.168.50.20 6443`
- Check k3s service logs on the joining node: `journalctl -u k3s -n 100`
- Verify `k3s_token` matches on all nodes: the token in group_vars must be identical across the cluster.

### NVMe not detected (`/dev/sda` missing)

- Verify the USB-NVMe enclosure is plugged in and powered.
- Run `lsblk` on the node directly to see available block devices.
- Check `dmesg | grep -i usb` for enumeration errors.
- The ping playbook (`00-ping.yml`) will hard-fail if `/dev/sda` is absent.

### Bootstrap fails mid-run (reboot required)

The `common` role may trigger a reboot (cgroup cmdline change). Ansible waits for the node to return (up to 300 seconds). If the node does not come back in time:

1. Check the node is actually rebooting.
2. Re-run `site.yml` — all tasks are idempotent.

### `cgroup_memory` not enabled after bootstrap

The cgroup parameters are appended to `/boot/firmware/cmdline.txt`. They take effect after a reboot, which is triggered automatically by the handler. If the reboot handler did not fire (e.g., task was skipped because cmdline.txt already had the values), verify with:

```bash
grep cgroup_memory /boot/firmware/cmdline.txt
```

### kubectl not found on rpi-0

k3s installs kubectl at `/usr/local/bin/kubectl`. Ensure `/usr/local/bin` is in PATH:

```bash
export PATH=$PATH:/usr/local/bin
kubectl get nodes
```

### Idempotency — safe to re-run

All playbooks are designed to be re-run safely:

- Package installation: `state: present` (no-op if installed).
- Partitioning: guarded by `stat` checks on partition existence.
- Formatting: guarded by `blkid` exit code.
- k3s install: guarded by `stat` on the systemd service file.
- Node labeling: uses `--overwrite`; reports `changed` only when value differs.
- SSH hardening: `lineinfile` and `blockinfile` are idempotent.
