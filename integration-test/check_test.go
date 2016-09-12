package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/docker/containerd/api/grpc/types"
	utils "github.com/docker/containerd/testutils"
	"github.com/go-check/check"
	"github.com/golang/protobuf/ptypes/timestamp"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

func init() {
	check.Suite(&ContainerdSuite{})
}

type ContainerdSuite struct {
	cwd               string
	outputDir         string
	stateDir          string
	grpcSocket        string
	logFile           *os.File
	cd                *exec.Cmd
	syncChild         chan error
	grpcClient        types.APIClient
	eventFiltersMutex sync.Mutex
	eventFilters      map[string]func(event *types.Event)
	lastEventTs       *timestamp.Timestamp
}

// getClient returns a connection to the Suite containerd
func (cs *ContainerdSuite) getClient(socket string) error {
	// Parse proto://address form addresses.
	bindParts := strings.SplitN(socket, "://", 2)
	if len(bindParts) != 2 {
		return fmt.Errorf("bad bind address format %s, expected proto://address", socket)
	}

	// reset the logger for grpc to log to dev/null so that it does not mess with our stdio
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout(bindParts[0], bindParts[1], timeout)
		}),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	conn, err := grpc.Dial(socket, dialOpts...)
	if err != nil {
		return err
	}
	healthClient := grpc_health_v1.NewHealthClient(conn)
	if _, err := healthClient.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{}); err != nil {
		return err
	}
	cs.grpcClient = types.NewAPIClient(conn)

	return nil
}

// ContainerdEventsHandler will process all events coming from
// containerd. If a filter as been register for a given container id
// via `SetContainerEventFilter()`, it will be invoked every time an
// event for that id is received
func (cs *ContainerdSuite) ContainerdEventsHandler(events types.API_EventsClient) {
	for {
		e, err := events.Recv()
		if err != nil {
			// If daemon died or exited, return
			if strings.Contains(err.Error(), "transport is closing") {
				break
			}
			time.Sleep(1 * time.Second)
			events, _ = cs.grpcClient.Events(context.Background(), &types.EventsRequest{Timestamp: cs.lastEventTs})
			continue
		}
		cs.lastEventTs = e.Timestamp
		cs.eventFiltersMutex.Lock()
		if f, ok := cs.eventFilters[e.Id]; ok {
			f(e)
			if e.Type == "exit" && e.Pid == "init" {
				delete(cs.eventFilters, e.Id)
			}
		}
		cs.eventFiltersMutex.Unlock()
	}
}

func (cs *ContainerdSuite) StopDaemon(kill bool) {
	if cs.cd == nil {
		return
	}

	if kill {
		cs.cd.Process.Kill()
		<-cs.syncChild
		cs.cd = nil
	} else {
		// Terminate gently if possible
		cs.cd.Process.Signal(os.Interrupt)

		done := false
		for done == false {
			select {
			case err := <-cs.syncChild:
				if err != nil {
					fmt.Printf("master containerd did not exit cleanly: %v\n", err)
				}
				done = true
			case <-time.After(3 * time.Second):
				fmt.Println("Timeout while waiting for containerd to exit, killing it!")
				cs.cd.Process.Kill()
			}
		}
	}
}

func (cs *ContainerdSuite) RestartDaemon(kill bool) error {
	cs.StopDaemon(kill)

	cd := exec.Command("containerd", "--debug",
		"--state-dir", cs.stateDir,
		"--listen", cs.grpcSocket,
		"--metrics-interval", "0m0s",
		"--runtime-args", fmt.Sprintf("--root=%s", filepath.Join(cs.cwd, cs.outputDir, "runc")),
	)
	cd.Stderr = cs.logFile
	cd.Stdout = cs.logFile

	if err := cd.Start(); err != nil {
		return err
	}
	cs.cd = cd

	if err := cs.getClient(cs.grpcSocket); err != nil {
		// Kill the daemon
		cs.cd.Process.Kill()
		return err
	}

	// Monitor events
	events, err := cs.grpcClient.Events(context.Background(), &types.EventsRequest{Timestamp: cs.lastEventTs})
	if err != nil {
		return err
	}

	go cs.ContainerdEventsHandler(events)

	go func() {
		cs.syncChild <- cd.Wait()
	}()

	return nil
}

func (cs *ContainerdSuite) SetUpSuite(c *check.C) {
	bundleMap = make(map[string]Bundle)
	cs.eventFilters = make(map[string]func(event *types.Event))

	// Get working directory for tests
	wd := utils.GetTestOutDir()
	if err := os.Chdir(wd); err != nil {
		c.Fatalf("Could not change working directory: %v", err)
	}
	cs.cwd = wd

	// Clean old bundles
	os.RemoveAll(utils.BundlesRoot)

	// Ensure the oci bundles directory exists
	if err := os.MkdirAll(utils.BundlesRoot, 0755); err != nil {
		c.Fatalf("Failed to create bundles directory: %v", err)
	}

	// Generate the reference spec
	if err := utils.GenerateReferenceSpecs(utils.BundlesRoot); err != nil {
		c.Fatalf("Unable to generate OCI reference spec: %v", err)
	}

	// Create our output directory
	cs.outputDir = fmt.Sprintf(utils.OutputDirFormat, time.Now().Format("2006-01-02_150405.000000"))

	cs.stateDir = filepath.Join(cs.outputDir, "containerd-master")
	if err := os.MkdirAll(cs.stateDir, 0755); err != nil {
		c.Fatalf("Unable to created output directory '%s': %v", cs.stateDir, err)
	}

	cs.grpcSocket = "unix://" + filepath.Join(cs.outputDir, "containerd-master", "containerd.sock")
	cdLogFile := filepath.Join(cs.outputDir, "containerd-master", "containerd.log")

	f, err := os.OpenFile(cdLogFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR|os.O_SYNC, 0777)
	if err != nil {
		c.Fatalf("Failed to create master containerd log file: %v", err)
	}
	cs.logFile = f

	cs.syncChild = make(chan error)
	cs.RestartDaemon(false)
}

func (cs *ContainerdSuite) TearDownSuite(c *check.C) {

	// tell containerd to stop
	if cs.cd != nil {
		cs.cd.Process.Signal(os.Interrupt)

		done := false
		for done == false {
			select {
			case err := <-cs.syncChild:
				if err != nil {
					c.Errorf("master containerd did not exit cleanly: %v", err)
				}
				done = true
			case <-time.After(3 * time.Second):
				fmt.Println("Timeout while waiting for containerd to exit, killing it!")
				cs.cd.Process.Kill()
			}
		}
	}

	if cs.logFile != nil {
		cs.logFile.Close()
	}
}

func (cs *ContainerdSuite) SetContainerEventFilter(id string, filter func(event *types.Event)) {
	cs.eventFiltersMutex.Lock()
	cs.eventFilters[id] = filter
	cs.eventFiltersMutex.Unlock()
}

func (cs *ContainerdSuite) TearDownTest(c *check.C) {
	ctrs, err := cs.ListRunningContainers()
	if err != nil {
		c.Fatalf("Unable to retrieve running containers: %v", err)
	}

	// Kill all containers that survived
	for _, ctr := range ctrs {
		ch := make(chan interface{})
		cs.SetContainerEventFilter(ctr.Id, func(e *types.Event) {
			if e.Type == "exit" && e.Pid == "init" {
				ch <- nil
			}
		})

		if err := cs.KillContainer(ctr.Id); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to cleanup leftover test containers: %v\n", err)
		}

		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			fmt.Fprintf(os.Stderr, "TearDownTest: Containerd %v didn't die after 3 seconds\n", ctr.Id)
		}
	}
}
