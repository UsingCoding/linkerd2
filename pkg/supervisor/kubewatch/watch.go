package kubewatch

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

type Watcher struct{}

type StartParams struct {
	Namespace string
	Pod       string
	Cancel    context.CancelFunc
}

func (w Watcher) Start(ctx context.Context, params StartParams) error {
	var readyPod bool

	ctx, cancelFunc := context.WithCancel(ctx)

	return w.startResourceWatch(
		ctx,
		params.Namespace,
		params.Pod,
		func(e watch.Event) {
			pod, ok := e.Object.(*corev1.Pod)
			if !ok {
				return
			}

			statuses := excludeLinkerdProxy(pod.Status.ContainerStatuses)

			if !readyPod {
				readyPod = allRunningAndReady(statuses)
				// wait next event
				return
			}

			if hasAnyContainerInTerminatedState(statuses) {
				// In those cases supervisor should not stop proxy
				// This happens when any of container in pod restarted by external reason (not by k8s kubelet initiative)

				// mark pod as not ready to wait for readiness of all containers
				readyPod = false
				return
			}

			if !allContainersNotReady(statuses) {
				// wait next event
				return
			}

			params.Cancel()
			cancelFunc()
		},
	)
}

type eventHandler func(e watch.Event)

func (w Watcher) startResourceWatch(
	ctx context.Context,
	namespace string,
	pod string,
	handler eventHandler,
) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return errors.Wrap(err, "failed to to get 'in cluster config'")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "failed to to create client")
	}

	fieldSelector := fields.OneTermEqualSelector("metadata.name", pod).String()

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (object runtime.Object, e error) {
			options.FieldSelector = fieldSelector
			return clientset.CoreV1().Pods(namespace).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (i watch.Interface, e error) {
			options.FieldSelector = fieldSelector
			return clientset.CoreV1().Pods(namespace).Watch(ctx, options)
		},
	}

	_, err = watchtools.UntilWithSync(
		ctx,
		lw,
		&corev1.Pod{}, // watch for pods
		nil,
		func(event watch.Event) (bool, error) {
			switch event.Type {
			case watch.Error:
				// unknown error happened on event
				return false, nil
			case watch.Deleted:
				// unreachable condition, handle for unpredictable k8s behavior
				return true, nil
			}

			handler(event)

			return false, nil
		},
	)

	if wait.Interrupted(err) {
		return nil
	}

	return err
}

func allRunningAndReady(containers []corev1.ContainerStatus) bool {
	for _, container := range containers {
		if container.State.Running == nil || !container.Ready {
			return false
		}
	}
	return true
}

// In deployment state of pod terminated containers means that they exited by other reasons than redeploy or restart of pod.
func hasAnyContainerInTerminatedState(containers []corev1.ContainerStatus) bool {
	for _, container := range containers {
		if container.State.Terminated != nil {
			return true
		}
	}
	return false
}

func allContainersNotReady(containers []corev1.ContainerStatus) bool {
	for _, container := range containers {
		if container.Ready {
			return false
		}
	}
	return true
}

func excludeLinkerdProxy(containers []corev1.ContainerStatus) []corev1.ContainerStatus {
	const linkerdContainer = "linkerd-proxy"
	res := make([]corev1.ContainerStatus, 0, len(containers))
	for _, container := range containers {
		if container.Name != linkerdContainer {
			res = append(res, container)
		}
	}
	return res
}
