package testutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetTestOutDir returns the output directory for testing and benchmark artifacts
func GetTestOutDir() string {
	out, _ := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	repoRoot := string(out)
	prefix := filepath.Join(strings.TrimSpace(repoRoot), "output")
	return prefix
}

var (
	// ArchivesDir holds the location of the available rootfs
	ArchivesDir = filepath.Join("test-artifacts", "archives")
	// BundlesRoot holds the location where OCI Bundles are stored
	BundlesRoot = filepath.Join("test-artifacts", "oci-bundles")
	// OutputDirFormat holds the standard format used when creating a
	// new test output directory
	OutputDirFormat = filepath.Join("test-artifacts", "runs", "%s")
	// RefOciSpecsPath holds the path to the generic OCI config
	RefOciSpecsPath = filepath.Join(BundlesRoot, "config.json")
	// StateDir holds the path to the directory used by the containerd
	// started by tests
	StateDir = "/run/containerd-bench-test"
)

// untarRootfs untars the given `source` tarPath into `destination/rootfs`
func untarRootfs(source string, destination string) error {
	rootfs := filepath.Join(destination, "rootfs")

	if err := os.MkdirAll(rootfs, 0755); err != nil {
		fmt.Println("untarRootfs os.MkdirAll failed with err %v", err)
		return nil
	}
	tar := exec.Command("tar", "-C", rootfs, "-xf", source)
	return tar.Run()
}

// GenerateReferenceSpecs generates a default OCI specs via `runc spec`
func GenerateReferenceSpecs(destination string) error {
	if _, err := os.Stat(filepath.Join(destination, "config.json")); err == nil {
		return nil
	}
	specs := exec.Command("runc", "spec")
	specs.Dir = destination
	return specs.Run()
}

// CreateBundle generates a valid OCI bundle from the given rootfs
func CreateBundle(source, name string) error {
	bundlePath := filepath.Join(BundlesRoot, name)

	if err := untarRootfs(filepath.Join(ArchivesDir, source+".tar"), bundlePath); err != nil {
		return fmt.Errorf("Failed to untar %s.tar: %v", source, err)
	}

	return nil
}

// CreateBusyboxBundle generates a bundle based on the busybox rootfs
func CreateBusyboxBundle(name string) error {
	return CreateBundle("busybox", name)
}
