// +build kube

/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/utils"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

// KubeClient API to access kube objects
type KubeClient struct {
	client    *kubernetes.Clientset
	namespace string
	config    *rest.Config
	ioStreams genericclioptions.IOStreams
}

// NewKubeClient new kubernetes client
func NewKubeClient(config genericclioptions.RESTClientGetter) (*KubeClient, error) {
	restConfig, err := config.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed creating clientset. Error: %+v", err)
	}

	namespace, _, err := config.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return nil, err
	}

	return &KubeClient{
		client:    clientset,
		namespace: namespace,
		config:    restConfig,
		ioStreams: genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
	}, nil
}

// GetContainers get containers for a given compose project
func (kc KubeClient) GetPod(ctx context.Context, projectName, serviceName string) (*corev1.Pod, error) {
	pods, err := kc.client.CoreV1().Pods(kc.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", compose.ProjectTag, projectName),
	})
	if err != nil {
		return nil, err
	}
	if pods == nil {
		return nil, nil
	}
	var pod corev1.Pod
	for _, p := range pods.Items {
		service := p.Labels[compose.ServiceTag]
		if service == serviceName {
			pod = p
			break
		}
	}
	return &pod, nil
}

// Exec executes a command in a container
func (kc KubeClient) Exec(ctx context.Context, projectName string, opts compose.RunOptions) error {
	pod, err := kc.GetPod(ctx, projectName, opts.Service)
	if err != nil || pod == nil {
		return err
	}
	if len(pod.Spec.Containers) == 0 {
		return fmt.Errorf("no containers running in pod %s", pod.Name)
	}
	// get first container in the pod
	container := &pod.Spec.Containers[0]
	containerName := container.Name

	req := kc.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(kc.namespace).
		SubResource("exec")

	option := &corev1.PodExecOptions{
		Container: containerName,
		Command:   opts.Command,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       opts.Tty,
	}

	if opts.Reader == nil {
		option.Stdin = false
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error adding to scheme: %v", err)
	}
	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(option, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(kc.config, "POST", req.URL())
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdin:  opts.Reader,
		Stdout: opts.Writer,
		Stderr: opts.Writer,
		Tty:    opts.Tty,
	})
}

// GetContainers get containers for a given compose project
func (kc KubeClient) GetContainers(ctx context.Context, projectName string, all bool) ([]compose.ContainerSummary, error) {
	fieldSelector := ""
	if !all {
		fieldSelector = "status.phase=Running"
	}

	pods, err := kc.client.CoreV1().Pods(kc.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", compose.ProjectTag, projectName),
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, err
	}
	result := []compose.ContainerSummary{}
	for _, pod := range pods.Items {
		result = append(result, podToContainerSummary(pod))
	}

	return result, nil
}

// GetLogs retrieves pod logs
func (kc *KubeClient) GetLogs(ctx context.Context, projectName string, consumer compose.LogConsumer, follow bool) error {
	pods, err := kc.client.CoreV1().Pods(kc.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", compose.ProjectTag, projectName),
	})
	if err != nil {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	for _, pod := range pods.Items {
		request := kc.client.CoreV1().Pods(kc.namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Follow: follow})
		service := pod.Labels[compose.ServiceTag]
		w := utils.GetWriter(pod.Name, service, func(event compose.ContainerEvent) {
			consumer.Log(event.Container, event.Service, event.Line)
		})

		eg.Go(func() error {
			r, err := request.Stream(ctx)
			if err != nil {
				return err
			}

			defer r.Close() // nolint errcheck
			_, err = io.Copy(w, r)
			return err
		})
	}
	return eg.Wait()
}

// WaitForPodState blocks until pods reach desired state
func (kc KubeClient) WaitForPodState(ctx context.Context, opts WaitForStatusOptions) error {
	var timeout = time.Minute
	if opts.Timeout != nil {
		timeout = *opts.Timeout
	}

	errch := make(chan error, 1)
	done := make(chan bool)
	go func() {
		for {
			time.Sleep(500 * time.Millisecond)

			pods, err := kc.client.CoreV1().Pods(kc.namespace).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", compose.ProjectTag, opts.ProjectName),
			})
			if err != nil {
				errch <- err
			}
			stateReached, servicePods, err := checkPodsState(opts.Services, pods.Items, opts.Status)
			if err != nil {
				errch <- err
			}
			if opts.Log != nil {
				for p, m := range servicePods {
					opts.Log(p, stateReached, m)
				}
			}

			if stateReached {
				done <- true
			}
		}
	}()

	select {
	case <-time.After(timeout):
		return fmt.Errorf("timeout: pods did not reach expected state")
	case err := <-errch:
		if err != nil {
			return err
		}
	case <-done:
		return nil
	}
	return nil
}

func (kc KubeClient) MapPorts(ctx context.Context, opts PortMappingOptions) error {

	stopChannel := make(chan struct{}, 1)
	readyChannel := make(chan struct{})

	eg, ctx := errgroup.WithContext(ctx)
	for serviceName, servicePorts := range opts.Services {
		serviceName = serviceName
		servicePorts = servicePorts
		pod, err := kc.GetPod(ctx, opts.ProjectName, serviceName)
		if err != nil {
			return err
		}
		eg.Go(func() error {

			req := kc.client.RESTClient().Post().Resource("pods").Namespace(kc.namespace).Name(pod.Name).SubResource("portforward") //fmt.Sprintf("service/%s", serviceName)).SubResource("portforward")
			transport, upgrader, err := spdy.RoundTripperFor(kc.config)
			if err != nil {
				return err
			}

			ports := []string{}
			for _, p := range servicePorts {
				ports = append(ports, fmt.Sprintf("%d:%d", p.PublishedPort, p.TargetPort))
			}
			//println(req.URL().String())
			//os.Exit(0)
			dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())
			fw, err := portforward.New(dialer, ports, stopChannel, readyChannel, os.Stdout, os.Stderr)
			if err != nil {
				return err
			}
			return fw.ForwardPorts()
		})
	}
	return eg.Wait()
}
