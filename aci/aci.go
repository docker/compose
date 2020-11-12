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

package aci

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	tm "github.com/buger/goterm"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/morikuni/aec"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/progress"
)

func createACIContainers(ctx context.Context, aciContext store.AciContext, groupDefinition containerinstance.ContainerGroup) error {
	containerGroupsClient, err := login.NewContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return errors.Wrapf(err, "cannot get container group client")
	}

	// Check if the container group already exists
	_, err = containerGroupsClient.Get(ctx, aciContext.ResourceGroup, *groupDefinition.Name)
	if err != nil {
		if err, ok := err.(autorest.DetailedError); ok {
			if err.StatusCode != http.StatusNotFound {
				return err
			}
		} else {
			return err
		}
	} else {
		return fmt.Errorf("container group %q already exists", *groupDefinition.Name)
	}

	return createOrUpdateACIContainers(ctx, aciContext, groupDefinition)
}

func createOrUpdateACIContainers(ctx context.Context, aciContext store.AciContext, groupDefinition containerinstance.ContainerGroup) error {
	w := progress.ContextWriter(ctx)
	containerGroupsClient, err := login.NewContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return errors.Wrapf(err, "cannot get container group client")
	}
	groupDisplay := "Group " + *groupDefinition.Name
	w.Event(progress.Event{
		ID:         groupDisplay,
		Status:     progress.Working,
		StatusText: "Waiting",
	})

	future, err := containerGroupsClient.CreateOrUpdate(
		ctx,
		aciContext.ResourceGroup,
		*groupDefinition.Name,
		groupDefinition,
	)
	if err != nil {
		return err
	}

	w.Event(progress.Event{
		ID:         groupDisplay,
		Status:     progress.Done,
		StatusText: "Created",
	})
	for _, c := range *groupDefinition.Containers {
		if c.Name != nil && *c.Name != convert.ComposeDNSSidecarName {
			w.Event(progress.Event{
				ID:         *c.Name,
				Status:     progress.Working,
				StatusText: "Waiting",
			})
		}
	}

	err = future.WaitForCompletionRef(ctx, containerGroupsClient.Client)
	if err != nil {
		return err
	}

	for _, c := range *groupDefinition.Containers {
		if c.Name != nil && *c.Name != convert.ComposeDNSSidecarName {
			w.Event(progress.Event{
				ID:         *c.Name,
				Status:     progress.Done,
				StatusText: "Done",
			})
		}
	}

	return err
}

func getACIContainerGroup(ctx context.Context, aciContext store.AciContext, containerGroupName string) (containerinstance.ContainerGroup, error) {
	containerGroupsClient, err := login.NewContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return containerinstance.ContainerGroup{}, fmt.Errorf("cannot get container group client: %v", err)
	}

	return containerGroupsClient.Get(ctx, aciContext.ResourceGroup, containerGroupName)
}

func getACIContainerGroups(ctx context.Context, subscriptionID string, resourceGroup string) ([]containerinstance.ContainerGroup, error) {
	groupsClient, err := login.NewContainerGroupsClient(subscriptionID)
	if err != nil {
		return nil, err
	}
	var containerGroups []containerinstance.ContainerGroup
	result, err := groupsClient.ListByResourceGroup(ctx, resourceGroup)
	if err != nil {
		return nil, err
	}

	for result.NotDone() {
		containerGroups = append(containerGroups, result.Values()...)
		if err := result.NextWithContext(ctx); err != nil {
			return nil, err
		}
	}
	var groups []containerinstance.ContainerGroup
	for _, group := range containerGroups {
		group, err := groupsClient.Get(ctx, resourceGroup, *group.Name)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func deleteACIContainerGroup(ctx context.Context, aciContext store.AciContext, containerGroupName string) (containerinstance.ContainerGroup, error) {
	containerGroupsClient, err := login.NewContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return containerinstance.ContainerGroup{}, fmt.Errorf("cannot get container group client: %v", err)
	}

	return containerGroupsClient.Delete(ctx, aciContext.ResourceGroup, containerGroupName)
}

func stopACIContainerGroup(ctx context.Context, aciContext store.AciContext, containerGroupName string) error {
	containerGroupsClient, err := login.NewContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return fmt.Errorf("cannot get container group client: %v", err)
	}

	result, err := containerGroupsClient.Stop(ctx, aciContext.ResourceGroup, containerGroupName)
	if result.IsHTTPStatus(http.StatusNotFound) {
		return errdefs.ErrNotFound
	}
	return err
}

func execACIContainer(ctx context.Context, aciContext store.AciContext, command, containerGroup string, containerName string) (c containerinstance.ContainerExecResponse, err error) {
	containerClient, err := login.NewContainerClient(aciContext.SubscriptionID)
	if err != nil {
		return c, errors.Wrapf(err, "cannot get container client")
	}
	rows, cols := getTermSize()
	containerExecRequest := containerinstance.ContainerExecRequest{
		Command: to.StringPtr(command),
		TerminalSize: &containerinstance.ContainerExecRequestTerminalSize{
			Rows: rows,
			Cols: cols,
		},
	}

	return containerClient.ExecuteCommand(
		ctx,
		aciContext.ResourceGroup,
		containerGroup,
		containerName,
		containerExecRequest)
}

func getTermSize() (*int32, *int32) {
	rows := tm.Height()
	cols := tm.Width()
	return to.Int32Ptr(int32(rows)), to.Int32Ptr(int32(cols))
}

func exec(ctx context.Context, address string, password string, request containers.ExecRequest) error {
	conn, _, _, err := ws.DefaultDialer.Dial(ctx, address)
	if err != nil {
		return err
	}
	err = wsutil.WriteClientMessage(conn, ws.OpText, []byte(password))
	if err != nil {
		return err
	}

	downstreamChannel := make(chan error, 10)
	upstreamChannel := make(chan error, 10)

	go func() {
		for {
			msg, _, err := wsutil.ReadServerData(conn)
			if err != nil {
				if err == io.EOF {
					downstreamChannel <- nil
					return
				}
				downstreamChannel <- err
				return
			}
			fmt.Fprint(request.Stdout, string(msg))
		}
	}()

	if request.Interactive {
		go func() {
			for {
				// We send each byte, byte-per-byte over the
				// websocket because the console is in raw mode
				buffer := make([]byte, 1)
				n, err := request.Stdin.Read(buffer)
				if err != nil {
					if err == io.EOF {
						upstreamChannel <- nil
						return
					}
					upstreamChannel <- err
					return
				}

				if n > 0 {
					err := wsutil.WriteClientMessage(conn, ws.OpText, buffer)
					if err != nil {
						upstreamChannel <- err
						return
					}
				}
			}
		}()
	}

	for {
		select {
		case err := <-downstreamChannel:
			return errors.Wrap(err, "failed to read input from container")
		case err := <-upstreamChannel:
			return errors.Wrap(err, "failed to send input to container")
		}
	}
}

func getACIContainerLogs(ctx context.Context, aciContext store.AciContext, containerGroupName, containerName string, tail *int32) (string, error) {
	containerClient, err := login.NewContainerClient(aciContext.SubscriptionID)
	if err != nil {
		return "", errors.Wrapf(err, "cannot get container client")
	}

	logs, err := containerClient.ListLogs(ctx, aciContext.ResourceGroup, containerGroupName, containerName, tail)
	if err != nil {
		return "", fmt.Errorf("cannot get container logs: %v", err)
	}
	if logs.Content == nil {
		return "", nil
	}
	return *logs.Content, err
}

func streamLogs(ctx context.Context, aciContext store.AciContext, containerGroupName, containerName string, req containers.LogsRequest) error {
	numLines := 0
	previousLogLines := ""
	firstDisplay := true // optimization to exit sooner in cases like docker run hello-world, do not wait another 2 secs.
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			logs, err := getACIContainerLogs(ctx, aciContext, containerGroupName, containerName, nil)
			if err != nil {
				return err
			}
			logLines := strings.Split(logs, "\n")
			currentOutput := len(logLines)

			// Note: a backend should not do this normally, this breaks the log
			// streaming over gRPC but this is the only thing we can do with
			// the kind of logs ACI is giving us. Hopefully Azue will give us
			// a real logs streaming api soon.
			b := aec.EmptyBuilder
			b = b.Up(uint(numLines))
			fmt.Fprint(req.Writer, b.Column(0).ANSI)

			numLines = getBacktrackLines(logLines, req.Width)

			for i := 0; i < currentOutput-1; i++ {
				fmt.Fprintln(req.Writer, logLines[i])
			}

			if (firstDisplay || previousLogLines == logs) && !isContainerRunning(ctx, aciContext, containerGroupName, containerName) {
				return nil
			}
			firstDisplay = false
			previousLogLines = logs

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func isContainerRunning(ctx context.Context, aciContext store.AciContext, containerGroupName, containerName string) bool {
	group, err := getACIContainerGroup(ctx, aciContext, containerGroupName)
	if err != nil {
		return false // group has disappeared
	}
	for _, container := range *group.Containers {
		if *container.Name == containerName {
			if convert.GetStatus(container, group) == convert.StatusRunning {
				return true
			}
		}
	}
	return false
}

func getBacktrackLines(lines []string, terminalWidth int) int {
	if terminalWidth == 0 { // no terminal width has been set, do not divide by zero
		return len(lines)
	}
	numLines := 0
	for i := 0; i < len(lines)-1; i++ {
		numLines++
		if len(lines[i]) > terminalWidth {
			numLines += len(lines[i]) / terminalWidth
		}
	}

	return numLines
}
