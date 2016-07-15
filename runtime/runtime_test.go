package runtime

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	utils "github.com/docker/containerd/testutils"
)

var (
	devNull     = "/dev/null"
	stdin       io.WriteCloser
	runtimeTool = flag.String("runtime", "runc", "Runtime to use for this test")
)

// Create containerd state and oci bundles directory
func setup() error {
	if err := os.MkdirAll(utils.StateDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(utils.BundlesRoot, 0755); err != nil {
		return err
	}
	return nil
}

// Creates the bundleDir with rootfs, io fifo dir and a default spec.
// On success, returns the bundlePath
func setupBundle(bundleName string) (string, error) {
	bundlePath := filepath.Join(utils.BundlesRoot, bundleName)
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		fmt.Println("Unable to create bundlePath due to ", err)
		return "", err
	}

	io := filepath.Join(bundlePath, "io")
	if err := os.MkdirAll(io, 0755); err != nil {
		fmt.Println("Unable to create io dir due to ", err)
		return "", err
	}

	if err := utils.GenerateReferenceSpecs(bundlePath); err != nil {
		fmt.Println("Unable to generate OCI reference spec: ", err)
		return "", err
	}

	if err := utils.CreateBusyboxBundle(bundleName); err != nil {
		fmt.Println("CreateBusyboxBundle error: ", err)
		return "", err
	}

	return bundlePath, nil
}

func setupStdio(cwd string, bundlePath string, bundleName string) (Stdio, error) {
	s := NewStdio(devNull, devNull, devNull)

	pid := "init"
	for stdName, stdPath := range map[string]*string{
		"stdin":  &s.Stdin,
		"stdout": &s.Stdout,
		"stderr": &s.Stderr,
	} {
		*stdPath = filepath.Join(cwd, bundlePath, "io", bundleName+"-"+pid+"-"+stdName)
		if err := syscall.Mkfifo(*stdPath, 0755); err != nil && !os.IsExist(err) {
			fmt.Println("Mkfifo error: ", err)
			return s, err
		}
	}

	err := attachStdio(s)
	if err != nil {
		fmt.Println("attachStdio error: ", err)
		return s, err
	}

	return s, nil
}

func attachStdio(s Stdio) error {
	stdinf, err := os.OpenFile(s.Stdin, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	stdin = stdinf
	stdoutf, err := os.OpenFile(s.Stdout, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	go io.Copy(os.Stdout, stdoutf)
	stderrf, err := os.OpenFile(s.Stderr, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	go io.Copy(os.Stderr, stderrf)
	return nil
}

func teardownBundle(bundleName string) {
	containerRoot := filepath.Join(utils.StateDir, bundleName)
	os.RemoveAll(containerRoot)

	bundlePath := filepath.Join(utils.BundlesRoot, bundleName)
	os.RemoveAll(bundlePath)
	return
}

// Remove containerd state and oci bundles directory
func teardown() {
	os.RemoveAll(utils.StateDir)
	os.RemoveAll(utils.BundlesRoot)
}

func BenchmarkBusyboxSh(b *testing.B) {
	bundleName := "busybox-sh"

	wd := utils.GetTestOutDir()
	if err := os.Chdir(wd); err != nil {
		b.Fatalf("Could not change working directory: %v", err)
	}

	if err := setup(); err != nil {
		b.Fatalf("Error setting up test: %v", err)
	}
	defer teardown()

	for n := 0; n < b.N; n++ {
		bundlePath, err := setupBundle(bundleName)
		if err != nil {
			return
		}

		s, err := setupStdio(wd, bundlePath, bundleName)
		if err != nil {
			return
		}

		c, err := New(ContainerOpts{
			Root:    utils.StateDir,
			ID:      bundleName,
			Bundle:  filepath.Join(wd, bundlePath),
			Runtime: *runtimeTool,
			Shim:    "containerd-shim",
			Timeout: 15 * time.Second,
		})

		if err != nil {
			b.Fatalf("Error creating a New container: ", err)
		}

		benchmarkStartContainer(b, c, s, bundleName)

		teardownBundle(bundleName)
	}
}

func benchmarkStartContainer(b *testing.B, c Container, s Stdio, bundleName string) {
	p, err := c.Start("", s)
	if err != nil {
		b.Fatalf("Error starting container %v", err)
	}

	kill := exec.Command(c.Runtime(), "kill", bundleName, "KILL")
	kill.Run()

	p.Wait()
	c.Delete()

	// wait for kill to finish. selected wait time is arbitrary
	time.Sleep(500 * time.Millisecond)

}
