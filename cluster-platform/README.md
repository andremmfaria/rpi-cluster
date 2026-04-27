# cluster-platform

GitOps-managed Kubernetes platform layer for the rpi-cluster.

Applied via `./bin/rpicli platform` from the repo root — no Flux or ArgoCD yet.

---

## Structure

```
cluster-platform/
└── infrastructure/
    ├── kube-vip/        # HA API endpoint (VIP: 192.168.50.30)
    ├── metallb/         # LoadBalancer IPs on LAN (192.168.50.40–60)
    ├── ingress-nginx/   # HTTP/S ingress controller (IP: 192.168.50.40)
    ├── longhorn/        # Replicated block storage on NVMe
    └── cert-manager/    # TLS certificate management (self-signed + Let's Encrypt DNS-01)
```

Scripts live at the repo root — see [`scripts/`](../scripts/) and [`bin/rpicli`](../bin/).

---

## Component versions

| Component     | Version        | How applied              |
| ------------- | -------------- | ------------------------ |
| kube-vip      | `v1.1.2`       | `kubectl apply -f`       |
| MetalLB       | `v0.15.3`      | two-step kubectl apply   |
| ingress-nginx | chart `4.15.1` | k3s `HelmChart` CRD      |
| Longhorn      | `1.11.1`       | k3s `HelmChart` CRD      |
| cert-manager  | `v1.20.2`      | k3s `HelmChart` CRD      |

---

## Getting started

### 1. Fetch kubeconfig

k3s writes its kubeconfig to `/etc/rancher/k3s/k3s.yaml` on the control-plane node.
Fetch it and rewrite the server address to the node IP:

```bash
./bin/rpicli platform kubeconfig \
  --server 192.168.50.20 \
  --user rpi \
  --key ~/.ssh/id_rpi
```

Saved to `~/.kube/rpi-cluster.yaml`. Export before running any other command:

```bash
export KUBECONFIG=~/.kube/rpi-cluster.yaml
```

After kube-vip is deployed, re-fetch pointing at the VIP:

```bash
./bin/rpicli platform kubeconfig --server 192.168.50.30
```

---

### 2. Populate secrets

Before applying cert-manager, create `config/secrets.toml` from the example:

```bash
cp config/secrets.toml.example config/secrets.toml
# edit config/secrets.toml — fill in your Cloudflare API token
```

`config/secrets.toml` is gitignored and never committed.

---

### 3. Apply components

Components must be applied in order — each depends on the previous.

```bash
./bin/rpicli platform apply all
```

Or one at a time:

```bash
./bin/rpicli platform apply kube-vip
./bin/rpicli platform apply metallb
./bin/rpicli platform apply ingress-nginx
./bin/rpicli platform apply longhorn
./bin/rpicli platform apply cert-manager
```

---

## Component details

### kube-vip

Floats the API VIP (`192.168.50.30:6443`) across `rpi-0`, `rpi-1`, `rpi-2` using ARP mode.
Runs as a DaemonSet scoped to control-plane nodes only.

Validate:

```bash
# One control-plane node should hold the VIP
ssh rpi@192.168.50.20 "ip a | grep 192.168.50.30"

# API reachable via VIP
kubectl --server=https://192.168.50.30:6443 get nodes
```

---

### MetalLB

Assigns real LAN IPs from `192.168.50.40–60` to `LoadBalancer` services.
Applied in two steps — upstream controller manifest first, then the `IPAddressPool` and `L2Advertisement` pool config once CRDs are established.

Validate:

```bash
kubectl get pods -n metallb-system
```

---

### ingress-nginx

Single ingress IP (`192.168.50.40`) routing HTTP/S by hostname. Installed via the k3s built-in HelmChart CRD — no Helm CLI needed.

Validate:

```bash
kubectl get svc -n ingress-nginx
# EXTERNAL-IP should be 192.168.50.40
```

---

### Longhorn

Replicated block storage across all 6 nodes. Data path `/mnt/nvme/longhorn` on each node's NVMe SSD. 2 replicas by default. Exposes a `longhorn` StorageClass as the cluster default.

Requires `open-iscsi` + `iscsid` running on all nodes — handled by the `cluster-setup` Ansible role.

```bash
./bin/rpicli platform apply longhorn
```

Validate:

```bash
kubectl get storageclass longhorn
kubectl get pods -n longhorn-system
kubectl get nodes.longhorn.io -n longhorn-system
```

UI: `http://longhorn.kantharos.srv` (after DNS entry pointing to `192.168.50.40`)

---

### cert-manager

Automatic TLS certificate management. Supports self-signed certs for internal use and Let's Encrypt DNS-01 via Cloudflare for LAN-only services.

**Before applying**, populate `config/secrets.toml` with your Cloudflare API token (see [Getting started](#2-populate-secrets)). The CLI reads the token at apply-time and substitutes it into `cloudflare-token-secret.yaml` before piping to `kubectl apply` — the real token never touches the repo.

```bash
./bin/rpicli platform apply cert-manager
```

Validate:

```bash
kubectl get clusterissuers
# NAME                    READY
# letsencrypt-prod        True
# letsencrypt-staging     True
# selfsigned              True
```

Test certificate (staging):

```bash
kubectl apply -f cluster-platform/infrastructure/cert-manager/test-certificate.yaml
kubectl describe certificate test-cert -n default
kubectl delete -f cluster-platform/infrastructure/cert-manager/test-certificate.yaml
```

Ingress annotation for automatic TLS:

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
    - hosts: [myapp.kantharos.srv]
      secretName: myapp-tls
```

---

## rpicli reference

All platform operations use `./bin/rpicli`. Run from the repo root.

### platform commands

| Command | Description |
| ------- | ----------- |
| `platform kubeconfig --server <ip>` | Fetch kubeconfig (user/key default from `cluster-config.toml`) |
| `platform apply <component\|all>` | Apply one or all components in order |
| `platform diff <component\|all>` | Dry-run diff against live cluster |
| `platform delete <component>` | Remove a component (requires confirmation) |
| `platform restart <component>` | Rollout restart a component's pods |
| `platform status [component]` | Show pods/services (default: all) |
| `platform logs <component\|pod>` | Stream logs with component auto-resolution |
| `platform events [-n ns\|-A]` | Show warning events sorted by time |
| `platform describe <type> [name]` | Describe a resource |
| `platform exec <component\|pod>` | Exec into a pod (default: sh) |
| `platform lint` | Run yamllint on cluster-platform manifests |

Known components: `kube-vip` · `metallb` · `ingress-nginx` · `longhorn` · `cert-manager`

### `scripts/act.sh`

| Command | Description |
| ------- | ----------- |
| `install` | Download and install act |
| `lint platform` | Run cluster-platform lint workflow |
| `lint all` | Run all lint workflows |

---

## Network map

| Endpoint             | Purpose                          |
| -------------------- | -------------------------------- |
| `192.168.50.30:6443` | Kubernetes API (HA via kube-vip) |
| `192.168.50.40`      | Ingress (nginx)                  |
| `192.168.50.40–60`   | MetalLB LoadBalancer pool        |

DNS entries (point to `192.168.50.40`):

```
longhorn.kantharos.srv
```

> cert-manager issues certificates via DNS-01 (Cloudflare). No DNS record required for cert-manager itself — certificates are requested per-ingress via annotation.
