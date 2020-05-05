package azure

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pkg/errors"

	"github.com/docker/api/context/store"

	tm "github.com/buger/goterm"
)

func init() {
	// required to get auth.NewAuthorizerFromCLI() to work, otherwise getting "The access token has been obtained for wrong audience or resource 'https://vault.azure.net'."
	err := os.Setenv("AZURE_KEYVAULT_RESOURCE", "https://management.azure.com")
	if err != nil {
		panic("unable to set environment variable AZURE_KEYVAULT_RESOURCE")
	}
}

func createACIContainers(ctx context.Context, aciContext store.AciContext, groupDefinition containerinstance.ContainerGroup) (c containerinstance.ContainerGroup, err error) {
	containerGroupsClient, err := getContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return c, errors.Wrapf(err, "cannot get container group client")
	}

	// Check if the container group already exists
	_, err = containerGroupsClient.Get(ctx, aciContext.ResourceGroup, *groupDefinition.Name)
	if err != nil {
		if err, ok := err.(autorest.DetailedError); ok {
			if err.StatusCode != http.StatusNotFound {
				return c, err
			}
		} else {
			return c, err
		}
	} else {
		return c, fmt.Errorf("container group %q already exists", *groupDefinition.Name)
	}

	future, err := containerGroupsClient.CreateOrUpdate(
		ctx,
		aciContext.ResourceGroup,
		*groupDefinition.Name,
		groupDefinition,
	)

	if err != nil {
		return c, err
	}

	err = future.WaitForCompletionRef(ctx, containerGroupsClient.Client)
	if err != nil {
		return c, err
	}
	containerGroup, err := future.Result(containerGroupsClient)
	if err != nil {
		return c, err
	}

	if len(*containerGroup.Containers) > 1 {
		var commands []string
		for _, container := range *containerGroup.Containers {
			commands = append(commands, fmt.Sprintf("echo 127.0.0.1 %s >> /etc/hosts", *container.Name))
		}
		commands = append(commands, "exit")

		containers := *containerGroup.Containers
		container := containers[0]
		response, err := execACIContainer(ctx, aciContext, "/bin/sh", *containerGroup.Name, *container.Name)
		if err != nil {
			return c, err
		}

		if err = execCommands(
			ctx,
			*response.WebSocketURI,
			*response.Password,
			commands,
		); err != nil {
			return containerinstance.ContainerGroup{}, err
		}
	}

	return containerGroup, err
}

func deleteACIContainerGroup(ctx context.Context, aciContext store.AciContext, containerGroupName string) (c containerinstance.ContainerGroup, err error) {
	containerGroupsClient, err := getContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return c, fmt.Errorf("cannot get container group client: %v", err)
	}
	return containerGroupsClient.Delete(ctx, aciContext.ResourceGroup, containerGroupName)
}

func execACIContainer(ctx context.Context, aciContext store.AciContext, command, containerGroup string, containerName string) (c containerinstance.ContainerExecResponse, err error) {
	containerClient, err := getContainerClient(aciContext.SubscriptionID)
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

type commandSender struct {
	commands []string
}

func (cs commandSender) Read(p []byte) (int, error) {
	if len(cs.commands) == 0 {
		return 0, io.EOF
	}
	command := cs.commands[0]
	cs.commands = cs.commands[1:]
	copy(p, command)
	return len(command), nil
}

func execCommands(ctx context.Context, address string, password string, commands []string) error {
	writer := ioutil.Discard
	reader := commandSender{
		commands: commands,
	}
	return exec(ctx, address, password, reader, writer)
}

func exec(ctx context.Context, address string, password string, reader io.Reader, writer io.Writer) error {
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
			fmt.Fprint(writer, string(msg))
		}
	}()

	go func() {
		for {
			// We send each byte, byte-per-byte over the
			// websocket because the console is in raw mode
			buffer := make([]byte, 1)
			n, err := reader.Read(buffer)
			if err != nil {
				upstreamChannel <- err
				return
			}

			if n > 0 {
				err := wsutil.WriteClientMessage(conn, ws.OpText, buffer)
				if err != nil {
					upstreamChannel <- err
				}
			}
		}
	}()

	for {
		select {
		case err := <-downstreamChannel:
			return errors.Wrap(err, "failed to read input from container")
		case err := <-upstreamChannel:
			return errors.Wrap(err, "failed to send input to container")
		}
	}
}

func getACIContainerLogs(ctx context.Context, aciContext store.AciContext, containerGroupName, containerName string) (string, error) {
	containerClient, err := getContainerClient(aciContext.SubscriptionID)
	if err != nil {
		return "", errors.Wrapf(err, "cannot get container client")
	}

	logs, err := containerClient.ListLogs(ctx, aciContext.ResourceGroup, containerGroupName, containerName, nil)
	if err != nil {
		return "", fmt.Errorf("cannot get container logs: %v", err)
	}
	return *logs.Content, err
}

func getContainerGroupsClient(subscriptionID string) (containerinstance.ContainerGroupsClient, error) {
	auth, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return containerinstance.ContainerGroupsClient{}, err
	}
	containerGroupsClient := containerinstance.NewContainerGroupsClient(subscriptionID)
	containerGroupsClient.Authorizer = auth
	return containerGroupsClient, nil
}

func getContainerClient(subscriptionID string) (containerinstance.ContainerClient, error) {
	auth, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return containerinstance.ContainerClient{}, err
	}
	containerClient := containerinstance.NewContainerClient(subscriptionID)
	containerClient.Authorizer = auth
	return containerClient, nil
}
