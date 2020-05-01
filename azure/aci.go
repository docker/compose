package azure

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/api/compose"
	"github.com/docker/api/context/store"
	"github.com/sirupsen/logrus"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"

	tm "github.com/buger/goterm"
)

const (
	AzureFileDriverName            = "azure_file"
	VolumeDriveroptsShareNameKey   = "share_name"
	VolumeDriveroptsAccountNameKey = "storage_account_name"
	VolumeDriveroptsAccountKeyKey  = "storage_account_key"
)
const singleContainerName = "single--container--aci"

func CreateACIContainers(ctx context.Context, project compose.Project, aciContext store.AciContext) (c containerinstance.ContainerGroup, err error) {
	containerGroupsClient, err := getContainerGroupsClient(aciContext.SubscriptionID)
	if err != nil {
		return c, fmt.Errorf("cannot get container group client: %v", err)
	}

	groupDefinition, err := convert(project, aciContext)
	if err != nil {
		return c, err
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

	if len(project.Services) > 1 {
		var commands []string
		for _, service := range project.Services {
			commands = append(commands, fmt.Sprintf("echo 127.0.0.1 %s >> /etc/hosts", service.Name))
		}
		commands = append(commands, "exit")

		response, err := ExecACIContainer(ctx, "/bin/sh", project.Name, project.Services[0].Name, aciContext)
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

type ProjectAciHelper compose.Project

func (p ProjectAciHelper) getAciSecretVolumes() ([]containerinstance.Volume, error) {
	var secretVolumes []containerinstance.Volume
	for secretName, filepathToRead := range p.Secrets {
		var data []byte
		if strings.HasPrefix(filepathToRead.File, compose.SecretInlineMark) {
			data = []byte(filepathToRead.File[len(compose.SecretInlineMark):])
		} else {
			var err error
			data, err = ioutil.ReadFile(filepathToRead.File)
			if err != nil {
				return secretVolumes, err
			}
		}
		if len(data) == 0 {
			continue
		}
		dataStr := base64.StdEncoding.EncodeToString(data)
		secretVolumes = append(secretVolumes, containerinstance.Volume{
			Name: to.StringPtr(secretName),
			Secret: map[string]*string{
				secretName: &dataStr,
			},
		})
	}
	return secretVolumes, nil
}

func (p ProjectAciHelper) getAciFileVolumes() (map[string]bool, []containerinstance.Volume, error) {
	azureFileVolumesMap := make(map[string]bool, len(p.Volumes))
	var azureFileVolumesSlice []containerinstance.Volume
	for name, v := range p.Volumes {
		if v.Driver == AzureFileDriverName {
			shareName, ok := v.DriverOpts[VolumeDriveroptsShareNameKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve share name for Azurefile")
			}
			accountName, ok := v.DriverOpts[VolumeDriveroptsAccountNameKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve account name for Azurefile")
			}
			accountKey, ok := v.DriverOpts[VolumeDriveroptsAccountKeyKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve account key for Azurefile")
			}
			aciVolume := containerinstance.Volume{
				Name: to.StringPtr(name),
				AzureFile: &containerinstance.AzureFileVolume{
					ShareName:          to.StringPtr(shareName),
					StorageAccountName: to.StringPtr(accountName),
					StorageAccountKey:  to.StringPtr(accountKey),
				},
			}
			azureFileVolumesMap[name] = true
			azureFileVolumesSlice = append(azureFileVolumesSlice, aciVolume)
		}
	}
	return azureFileVolumesMap, azureFileVolumesSlice, nil
}

type ServiceConfigAciHelper types.ServiceConfig

func (s ServiceConfigAciHelper) getAciFileVolumeMounts(volumesCache map[string]bool) ([]containerinstance.VolumeMount, error) {
	var aciServiceVolumes []containerinstance.VolumeMount
	for _, sv := range s.Volumes {
		if !volumesCache[sv.Source] {
			return []containerinstance.VolumeMount{}, fmt.Errorf("could not find volume source %q", sv.Source)
		}
		aciServiceVolumes = append(aciServiceVolumes, containerinstance.VolumeMount{
			Name:      to.StringPtr(sv.Source),
			MountPath: to.StringPtr(sv.Target),
		})
	}
	return aciServiceVolumes, nil
}

func (s ServiceConfigAciHelper) getAciSecretsVolumeMounts() []containerinstance.VolumeMount {
	var secretVolumeMounts []containerinstance.VolumeMount
	for _, secret := range s.Secrets {
		secretsMountPath := "/run/secrets"
		if secret.Target == "" {
			secret.Target = secret.Source
		}
		// Specifically use "/" here and not filepath.Join() to avoid windows path being sent and used inside containers
		secretsMountPath = secretsMountPath + "/" + secret.Target
		vmName := strings.Split(secret.Source, "=")[0]
		vm := containerinstance.VolumeMount{
			Name:      to.StringPtr(vmName),
			MountPath: to.StringPtr(secretsMountPath),
			ReadOnly:  to.BoolPtr(true), // TODO Confirm if the secrets are read only
		}
		secretVolumeMounts = append(secretVolumeMounts, vm)
	}
	return secretVolumeMounts
}

func (s ServiceConfigAciHelper) getAciContainer(volumesCache map[string]bool) (containerinstance.Container, error) {
	secretVolumeMounts := s.getAciSecretsVolumeMounts()
	aciServiceVolumes, err := s.getAciFileVolumeMounts(volumesCache)
	if err != nil {
		return containerinstance.Container{}, err
	}
	allVolumes := append(aciServiceVolumes, secretVolumeMounts...)
	var volumes *[]containerinstance.VolumeMount
	if len(allVolumes) == 0 {
		volumes = nil
	} else {
		volumes = &allVolumes
	}
	return containerinstance.Container{
		Name: to.StringPtr(s.Name),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image: to.StringPtr(s.Image),
			Resources: &containerinstance.ResourceRequirements{
				Limits: &containerinstance.ResourceLimits{
					MemoryInGB: to.Float64Ptr(1),
					CPU:        to.Float64Ptr(1),
				},
				Requests: &containerinstance.ResourceRequests{
					MemoryInGB: to.Float64Ptr(1),
					CPU:        to.Float64Ptr(1),
				},
			},
			VolumeMounts: volumes,
		},
	}, nil
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

func convert(p compose.Project, aciContext store.AciContext) (containerinstance.ContainerGroup, error) {
	project := ProjectAciHelper(p)
	containerGroupName := strings.ToLower(project.Name)
	volumesCache, volumesSlice, err := project.getAciFileVolumes()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	secretVolumes, err := project.getAciSecretVolumes()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	allVolumes := append(volumesSlice, secretVolumes...)
	var volumes *[]containerinstance.Volume
	if len(allVolumes) == 0 {
		volumes = nil
	} else {
		volumes = &allVolumes
	}
	var containers []containerinstance.Container
	groupDefinition := containerinstance.ContainerGroup{
		Name:     &containerGroupName,
		Location: &aciContext.Location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType:     containerinstance.Linux,
			Containers: &containers,
			Volumes:    volumes,
		},
	}

	for _, s := range project.Services {
		service := ServiceConfigAciHelper(s)
		if s.Name != singleContainerName {
			logrus.Debugf("Adding %q\n", service.Name)
		}
		containerDefinition, err := service.getAciContainer(volumesCache)
		if err != nil {
			return containerinstance.ContainerGroup{}, err
		}
		if service.Ports != nil {
			var containerPorts []containerinstance.ContainerPort
			var groupPorts []containerinstance.Port
			for _, portConfig := range service.Ports {
				if portConfig.Published != 0 && portConfig.Published != portConfig.Target {
					msg := fmt.Sprintf("Port mapping is not supported with ACI, cannot map port %d to %d for container %s",
						portConfig.Published, portConfig.Target, service.Name)
					return groupDefinition, errors.New(msg)
				}
				portNumber := int32(portConfig.Target)
				containerPorts = append(containerPorts, containerinstance.ContainerPort{
					Port: to.Int32Ptr(portNumber),
				})
				groupPorts = append(groupPorts, containerinstance.Port{
					Port:     to.Int32Ptr(portNumber),
					Protocol: containerinstance.TCP,
				})
			}
			containerDefinition.ContainerProperties.Ports = &containerPorts
			groupDefinition.ContainerGroupProperties.IPAddress = &containerinstance.IPAddress{
				Type:  containerinstance.Public,
				Ports: &groupPorts,
			}
		}

		containers = append(containers, containerDefinition)
	}
	groupDefinition.ContainerGroupProperties.Containers = &containers
	return groupDefinition, nil
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
