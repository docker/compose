package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/go-check/check"
)

var (
	outputDirFormat = filepath.Join("test-artifacts", "runs", "%s")
	archivesDir     = filepath.Join("test-artifacts", "archives")
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
	logFile           *os.File
	cd                *exec.Cmd
	syncChild         chan error
	grpcClient        types.APIClient
	eventFiltersMutex sync.Mutex
	eventFilters      map[string]func(event *types.Event)
}

// getClient returns a connection to the Suite containerd
func (cs *ContainerdSuite) getClient(socket string) error {
	// reset the logger for grpc to log to dev/null so that it does not mess with our stdio
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		},
		))
	conn, err := grpc.Dial(socket, dialOpts...)
	if err != nil {
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
	timestamp := uint64(time.Now().Unix())
	for {
		e, err := events.Recv()
		if err != nil {
			time.Sleep(1 * time.Second)
			events, _ = cs.grpcClient.Events(context.Background(), &types.EventsRequest{Timestamp: timestamp})
			continue
		}
		timestamp = e.Timestamp
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

// generateReferencesSpecs invoke `runc spec` to produce the baseline
// specs from which all future bundle will be generated
func generateReferenceSpecs(destination string) error {
	specs := exec.Command("runc", "spec")
	specs.Dir = destination
	return specs.Run()
}

func (cs *ContainerdSuite) SetUpSuite(c *check.C) {
	bundleMap = make(map[string]Bundle)
	cs.eventFilters = make(map[string]func(event *types.Event))

	// Get our CWD
	if cwd, err := os.Getwd(); err != nil {
		c.Fatalf("Could not determine current working directory: %v", err)
	} else {
		cs.cwd = cwd
	}

	// Clean old bundles
	os.RemoveAll(bundlesDir)

	// Ensure the oci bundles directory exists
	if err := os.MkdirAll(bundlesDir, 0755); err != nil {
		c.Fatalf("Failed to create bundles directory: %v", err)
	}

	// Generate the reference spec
	if err := generateReferenceSpecs(bundlesDir); err != nil {
		c.Fatalf("Unable to generate OCI reference spec: %v", err)
	}

	// Create our output directory
	od := fmt.Sprintf(outputDirFormat, time.Now().Format("2006-01-02_150405.000000"))
	cdStateDir := fmt.Sprintf("%s/containerd-master", od)
	if err := os.MkdirAll(cdStateDir, 0755); err != nil {
		c.Fatalf("Unable to created output directory '%s': %v", cdStateDir, err)
	}

	cdGRPCSock := filepath.Join(od, "containerd-master", "containerd.sock")
	cdLogFile := filepath.Join(od, "containerd-master", "containerd.log")

	f, err := os.OpenFile(cdLogFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR|os.O_SYNC, 0777)
	if err != nil {
		c.Fatalf("Failed to create master containerd log file: %v", err)
	}
	cs.logFile = f

	cd := exec.Command("containerd", "--debug",
		"--state-dir", cdStateDir,
		"--listen", cdGRPCSock,
		"--metrics-interval", "0m0s",
		"--runtime-args", fmt.Sprintf("--root=%s", filepath.Join(cs.cwd, cdStateDir, "runc")),
	)
	cd.Stderr = f
	cd.Stdout = f

	if err := cd.Start(); err != nil {
		c.Fatalf("Unable to start the master containerd: %v", err)
	}

	cs.outputDir = od
	cs.cd = cd
	cs.syncChild = make(chan error)
	if err := cs.getClient(cdGRPCSock); err != nil {
		// Kill the daemon
		cs.cd.Process.Kill()
		c.Fatalf("Failed to connect to daemon: %v", err)
	}

	// Monitor events
	events, err := cs.grpcClient.Events(context.Background(), &types.EventsRequest{})
	if err != nil {
		c.Fatalf("Could not register containerd event handler: %v", err)
	}

	go cs.ContainerdEventsHandler(events)

	go func() {
		cs.syncChild <- cd.Wait()
	}()
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
			fmt.Fprintf(os.Stderr, "Failed to cleanup leftover test containers: %v", err)
		}

		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			fmt.Fprintf(os.Stderr, "TearDownTest: Containerd %v didn't die after 3 seconds", ctr.Id)
		}
	}
}
