package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetKnownComponent(t *testing.T) {
	c, err := Get("longhorn")
	require.NoError(t, err)
	assert.Equal(t, "longhorn", c.Name)
	assert.Equal(t, "longhorn-system", c.Namespace)
	assert.Equal(t, "app=longhorn-manager", c.Selector)
}

func TestGetAllComponents(t *testing.T) {
	for _, name := range []string{"kube-vip", "metallb", "ingress-nginx", "longhorn"} {
		t.Run(name, func(t *testing.T) {
			c, err := Get(name)
			require.NoError(t, err)
			assert.NotEmpty(t, c.Namespace)
			assert.NotEmpty(t, c.Selector)
			assert.NotEmpty(t, c.Workloads)
		})
	}
}

func TestGetUnknownComponent(t *testing.T) {
	_, err := Get("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown component")
}

func TestAllReturnsCorrectOrder(t *testing.T) {
	all := All()
	require.Len(t, all, 5)
	assert.Equal(t, "kube-vip", all[0].Name)
	assert.Equal(t, "metallb", all[1].Name)
	assert.Equal(t, "ingress-nginx", all[2].Name)
	assert.Equal(t, "longhorn", all[3].Name)
	assert.Equal(t, "cert-manager", all[4].Name)
}

func TestMetalLBHasTwoWorkloads(t *testing.T) {
	c, err := Get("metallb")
	require.NoError(t, err)
	assert.Len(t, c.Workloads, 2)
}

func TestWorkloadKinds(t *testing.T) {
	c, err := Get("kube-vip")
	require.NoError(t, err)
	assert.Equal(t, "daemonset", c.Workloads[0].Kind)
}
