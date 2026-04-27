package component

import (
	"fmt"
)

type Workload struct {
	Kind string
	Name string
}

type Component struct {
	Name      string
	Namespace string
	Selector  string
	Workloads []Workload
}

var registry = map[string]Component{
	"kube-vip": {
		Name:      "kube-vip",
		Namespace: "kube-system",
		Selector:  "app=kube-vip",
		Workloads: []Workload{{Kind: "daemonset", Name: "kube-vip"}},
	},
	"metallb": {
		Name:      "metallb",
		Namespace: "metallb-system",
		Selector:  "app=metallb",
		Workloads: []Workload{
			{Kind: "deployment", Name: "controller"},
			{Kind: "daemonset", Name: "speaker"},
		},
	},
	"ingress-nginx": {
		Name:      "ingress-nginx",
		Namespace: "ingress-nginx",
		Selector:  "app.kubernetes.io/name=ingress-nginx",
		Workloads: []Workload{{Kind: "deployment", Name: "ingress-nginx-controller"}},
	},
	"longhorn": {
		Name:      "longhorn",
		Namespace: "longhorn-system",
		Selector:  "app=longhorn-manager",
		Workloads: []Workload{
			{Kind: "daemonset", Name: "longhorn-manager"},
			{Kind: "daemonset", Name: "longhorn-csi-plugin"},
		},
	},
}

func Get(name string) (Component, error) {
	c, ok := registry[name]
	if !ok {
		return Component{}, fmt.Errorf("unknown component %q — valid: kube-vip, metallb, ingress-nginx, longhorn", name)
	}
	return c, nil
}

func All() []Component {
	order := []string{"kube-vip", "metallb", "ingress-nginx", "longhorn"}
	out := make([]Component, 0, len(order))
	for _, name := range order {
		out = append(out, registry[name])
	}
	return out
}
