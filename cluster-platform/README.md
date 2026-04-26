# cluster-platform

GitOps-managed Kubernetes platform layer for the rpi-cluster.

Applied manually with `kubectl apply` / `kubectl apply -k` — no Flux or ArgoCD yet.

---

## Structure

```
cluster-platform/
└── infrastructure/
    ├── kube-vip/        # HA API endpoint (VIP: 192.168.50.30)
    ├── metallb/         # LoadBalancer IPs on LAN (192.168.50.40–60)
    └── ingress-nginx/   # HTTP/S ingress controller (IP: 192.168.50.40)
```

---

## Apply order

Components must be applied in this exact order. Each depends on the previous.

### 1. kube-vip

Floats the API VIP (`192.168.50.30:6443`) across control-plane nodes. Runs as a DaemonSet on `rpi-0`, `rpi-1`, `rpi-2` only.

```bash
kubectl apply -f infrastructure/kube-vip/
```

Validate:

```bash
# One control-plane node should hold the VIP
ssh rpi@192.168.50.20 "ip a | grep 192.168.50.30"

# API reachable via VIP
kubectl --server=https://192.168.50.30:6443 get nodes
```

---

### 2. MetalLB

Assigns real LAN IPs to `LoadBalancer` services from the pool `192.168.50.40–60`.

```bash
# Requires kubectl with kustomize support (kubectl >= 1.14)
kubectl apply -k infrastructure/metallb/
```

Validate:

```bash
kubectl get pods -n metallb-system
```

---

### 3. ingress-nginx

Single ingress IP (`192.168.50.40`) routing HTTP/S traffic to services by hostname. Installed via k3s HelmChart CRD.

```bash
kubectl apply -f infrastructure/ingress-nginx/
```

Validate:

```bash
kubectl get svc -n ingress-nginx
# EXTERNAL-IP should be 192.168.50.40
```

---

## Network map

| Endpoint              | Purpose                        |
| --------------------- | ------------------------------ |
| `192.168.50.30:6443`  | Kubernetes API (HA via kube-vip) |
| `192.168.50.40`       | Ingress (nginx)                |
| `192.168.50.40–60`    | MetalLB LoadBalancer pool      |

---

## DNS (minimal, pre-cert-manager)

Point these at `192.168.50.40` in AdGuard / router:

```
rancher.lab
grafana.lab
longhorn.lab
```
