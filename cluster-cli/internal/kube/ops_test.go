package kube

import (
	"bytes"
	"context"
	"testing"
	"time"

	"rpicli/internal/component"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func runningPod(name, namespace string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "test"}},
		},
	}
}

func TestPrintStatusSingleComponent(t *testing.T) {
	client := fake.NewSimpleClientset(
		runningPod("longhorn-manager-abc", "longhorn-system", map[string]string{"app": "longhorn-manager"}),
		runningPod("longhorn-manager-def", "longhorn-system", map[string]string{"app": "longhorn-manager"}),
	)

	var buf bytes.Buffer
	err := PrintStatus(context.Background(), client, "longhorn")
	assert.NoError(t, err)
	_ = buf
}

func TestPrintStatusUnknownComponent(t *testing.T) {
	client := fake.NewSimpleClientset()
	err := PrintStatus(context.Background(), client, "unknown-component")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown component")
}

func TestResolvePodsByComponent(t *testing.T) {
	client := fake.NewSimpleClientset(
		runningPod("kube-vip-abc", "kube-system", map[string]string{"app": "kube-vip"}),
		runningPod("kube-vip-def", "kube-system", map[string]string{"app": "kube-vip"}),
	)

	pods, ns, err := resolvePods(context.Background(), client, "kube-vip", "")
	require.NoError(t, err)
	assert.Equal(t, "kube-system", ns)
	assert.Len(t, pods, 2)
}

func TestResolvePodDirectNameRequiresNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	_, _, err := resolvePods(context.Background(), client, "my-pod", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--namespace")
}

func TestResolvePodDirectNameWithNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	pods, ns, err := resolvePods(context.Background(), client, "my-pod", "default")
	require.NoError(t, err)
	assert.Equal(t, "default", ns)
	assert.Equal(t, []string{"my-pod"}, pods)
}

func TestCordonUncordonNode(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "rpi-0"},
		Spec:       corev1.NodeSpec{Unschedulable: false},
	}
	client := fake.NewSimpleClientset(node)
	ctx := context.Background()

	require.NoError(t, CordonNode(ctx, client, "rpi-0"))
	n, _ := client.CoreV1().Nodes().Get(ctx, "rpi-0", metav1.GetOptions{})
	assert.True(t, n.Spec.Unschedulable)

	require.NoError(t, UncordonNode(ctx, client, "rpi-0"))
	n, _ = client.CoreV1().Nodes().Get(ctx, "rpi-0", metav1.GetOptions{})
	assert.False(t, n.Spec.Unschedulable)
}

func TestNodeStatus(t *testing.T) {
	n := corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	assert.Equal(t, "Ready", nodeStatus(n))

	n.Spec.Unschedulable = true
	assert.Equal(t, "Ready,SchedulingDisabled", nodeStatus(n))
}

func TestRolloutRestartUpdatesAnnotation(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-vip", Namespace: "kube-system"},
	}
	client := fake.NewSimpleClientset(ds)
	comp, _ := component.Get("kube-vip")

	err := RolloutRestart(context.Background(), client, comp)
	assert.NoError(t, err)

	updated, _ := client.AppsV1().DaemonSets("kube-system").Get(context.Background(), "kube-vip", metav1.GetOptions{})
	assert.NotEmpty(t, updated.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"])
}

func TestFormatAge(t *testing.T) {
	assert.Equal(t, "30s", formatAge(time.Now().Add(-30*time.Second)))
	assert.Equal(t, "5m", formatAge(time.Now().Add(-5*time.Minute)))
	assert.Equal(t, "2h", formatAge(time.Now().Add(-2*time.Hour)))
	assert.Equal(t, "3d", formatAge(time.Now().Add(-72*time.Hour)))
}

func TestSortEventsByTime(t *testing.T) {
	t1 := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	t2 := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	t3 := metav1.NewTime(time.Now())

	events := []corev1.Event{
		{LastTimestamp: t3},
		{LastTimestamp: t1},
		{LastTimestamp: t2},
	}
	sortEventsByTime(events)

	assert.Equal(t, t1, events[0].LastTimestamp)
	assert.Equal(t, t2, events[1].LastTimestamp)
	assert.Equal(t, t3, events[2].LastTimestamp)
}
