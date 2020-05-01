package azure

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/docker/api/context/store"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"

	tm "github.com/buger/goterm"
)

func init() {
	// required to get auth.NewAuthorizerFromCLI() work, otherwise getting "The access token has been obtained for wrong audience or resource 'https://vault.azure.net'."
	_ = os.Setenv("AZURE_KEYVAULT_RESOURCE", "https://management.azure.com")
}

func CreateACIContainers(ctx context.Context, aciContext store.AciContext, groupDefinition containerinstance.ContainerGroup) (c containerinstance.ContainerGroup, err error) {
	containerGroupsClient, err := getContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return c, fmt.Errorf("cannot get container group client: %v", err)
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
		return c, fmt.Errorf("Container group %q already exists", *groupDefinition.Name)
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
		response, err := ExecACIContainer(ctx, "/bin/sh", *containerGroup.Name, *container.Name, aciContext)
		if err != nil {
			return c, err
		}

		err = ExecWebSocketLoopWithCmd(
			ctx,
			*response.WebSocketURI,
			*response.Password,
			commands,
			false)
		if err != nil {
			return containerinstance.ContainerGroup{}, err
		}
	}

	return containerGroup, err
}

// ListACIContainers List available containers
func ListACIContainers(aciContext store.AciContext) (c []containerinstance.ContainerGroup, err error) {
	ctx := context.TODO()
	containerGroupsClient, err := getContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return c, fmt.Errorf("cannot get container group client: %v", err)
	}

	var containers []containerinstance.ContainerGroup
	result, err := containerGroupsClient.ListByResourceGroup(ctx, aciContext.ResourceGroup)
	if err != nil {
		return []containerinstance.ContainerGroup{}, err
	}
	for result.NotDone() {
		containers = append(containers, result.Values()...)
		if err := result.NextWithContext(ctx); err != nil {
			return []containerinstance.ContainerGroup{}, err
		}
	}

	return containers, err
}

func ExecACIContainer(ctx context.Context, command, containerGroup string, containerName string, aciContext store.AciContext) (c containerinstance.ContainerExecResponse, err error) {
	containerClient := getContainerClient(aciContext.SubscriptionID)
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

func ExecWebSocketLoop(ctx context.Context, wsURL, passwd string) error {
	return ExecWebSocketLoopWithCmd(ctx, wsURL, passwd, []string{}, true)
}

func ExecWebSocketLoopWithCmd(ctx context.Context, wsURL, passwd string, commands []string, outputEnabled bool) error {
	ctx, cancel := context.WithCancel(ctx)
	conn, _, _, err := ws.DefaultDialer.Dial(ctx, wsURL)
	if err != nil {
		cancel()
		return err
	}
	err = wsutil.WriteClientMessage(conn, ws.OpText, []byte(passwd))
	if err != nil {
		cancel()
		return err
	}
	lastCommandLen := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, _, err := wsutil.ReadServerData(conn)
			if err != nil {
				if err != io.EOF {
					fmt.Printf("read error: %s\n", err)
				}
				return
			}
			lines := strings.Split(string(msg), "\n")
			lastCommandLen = len(lines[len(lines)-1])
			if outputEnabled {
				fmt.Printf("%s", msg)
			}
		}
	}()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	scanner := bufio.NewScanner(os.Stdin)
	rc := make(chan string, 10)
	if len(commands) > 0 {
		for _, command := range commands {
			rc <- command
		}
	}
	go func() {
		for {
			if !scanner.Scan() {
				close(done)
				cancel()
				fmt.Println("exiting...")
				break
			}
			t := scanner.Text()
			rc <- t
			cleanLastCommand(lastCommandLen)
		}
	}()
	for {
		select {
		case <-done:
			return nil
		case line := <-rc:
			err = wsutil.WriteClientMessage(conn, ws.OpText, []byte(line+"\n"))
			if err != nil {
				fmt.Println("write: ", err)
				return nil
			}
		case <-interrupt:
			fmt.Println("interrupted...")
			close(done)
			cancel()
			return nil
		}
	}
}

func cleanLastCommand(lastCommandLen int) {
	tm.MoveCursorUp(1)
	tm.MoveCursorForward(lastCommandLen)
	if runtime.GOOS != "windows" {
		for i := 0; i < tm.Width(); i++ {
			_, _ = tm.Print(" ")
		}
		tm.MoveCursorUp(1)
	}

	tm.Flush()
}

func getContainerGroupsClient(subscriptionID string) (containerinstance.ContainerGroupsClient, error) {
	auth, _ := auth.NewAuthorizerFromCLI()
	containerGroupsClient := containerinstance.NewContainerGroupsClient(subscriptionID)
	containerGroupsClient.Authorizer = auth
	return containerGroupsClient, nil
}

func getContainerClient(subscriptionID string) containerinstance.ContainerClient {
	auth, _ := auth.NewAuthorizerFromCLI()
	containerClient := containerinstance.NewContainerClient(subscriptionID)
	containerClient.Authorizer = auth
	return containerClient
}
