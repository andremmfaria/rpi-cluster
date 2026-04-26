# cluster-platform

GitOps-managed Kubernetes platform layer for the rpi-cluster.

Applied via `scripts/run.sh` — no Flux or ArgoCD yet.

---

## Structure

```
cluster-platform/
├── infrastructure/
│   ├── kube-vip/        # HA API endpoint (VIP: 192.168.50.30)
│   ├── metallb/         # LoadBalancer IPs on LAN (192.168.50.40–60)
│   ├── ingress-nginx/   # HTTP/S ingress controller (IP: 192.168.50.40)
│   └── longhorn/        # Replicated block storage on NVMe
└── scripts/
    ├── run.sh           # kubectl wrapper (kubeconfig/apply/diff/delete/status/lint)
    └── act.sh           # Local CI runner via act
```

---

## Component versions

| Component     | Version        | How applied              |
| ------------- | -------------- | ------------------------ |
| kube-vip      | `v1.1.2`       | `kubectl apply -f`       |
| MetalLB       | `v0.15.3`      | `kubectl apply -k`       |
| ingress-nginx | chart `4.15.1` | k3s `HelmChart` CRD      |
| Longhorn      | `1.11.1`       | k3s `HelmChart` CRD      |

---

## Getting started

### 1. Fetch kubeconfig

k3s writes its kubeconfig to `/etc/rancher/k3s/k3s.yaml` on the control-plane node.
Fetch it and rewrite the server address to the node IP:

```bash
./scripts/run.sh kubeconfig \
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
./scripts/run.sh kubeconfig \
  --server 192.168.50.30 \
  --user rpi \
  --key ~/.ssh/id_rpi
```

---

### 2. Apply components

Components must be applied in order — each depends on the previous.

```bash
./scripts/run.sh apply all
```

Or one at a time:

```bash
./scripts/run.sh apply kube-vip
./scripts/run.sh apply metallb
./scripts/run.sh apply ingress-nginx
./scripts/run.sh apply longhorn
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
./scripts/run.sh apply longhorn
```

Validate:

```bash
kubectl get storageclass longhorn
kubectl get pods -n longhorn-system
kubectl get nodes.longhorn.io -n longhorn-system
```

UI: `http://longhorn.kantharos.srv` (after DNS entry pointing to `192.168.50.40`)

Test dynamic provisioning:

```bash
kubectl apply -f infrastructure/longhorn/test-pvc.yaml
kubectl get pvc longhorn-test-pvc
kubectl delete -f infrastructure/longhorn/test-pvc.yaml
```

---

## Scripts reference

### `scripts/run.sh`

| Command | Description |
| ------- | ----------- |
| `kubeconfig --server <ip> --user <user> --key <key>` | Fetch kubeconfig from a control-plane node |
| `apply <component\|all>` | Apply one or all components in order |
| `diff <component\|all>` | Dry-run diff against live cluster |
| `delete <component>` | Remove a component (requires confirmation) |
| `status` | Show pods/services for all platform namespaces |
| `lint` | Run yamllint on all manifests |

### `scripts/act.sh`

Runs the `cluster-platform-lint.yml` CI workflow locally via [act](https://github.com/nektos/act).

| Command | Description |
| ------- | ----------- |
| `install` | Download and install act |
| `lint` | Run the cluster-platform lint workflow |
| `all` | Same as lint (only one workflow in this module) |

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
