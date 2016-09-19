package supervisor

import (
	"flag"
	"os"
	"sort"
	"testing"

	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/specs"
)

var (
	runtimeTool = flag.String("runtime", "runc", "Runtime to use for this test")
)

type testProcess struct {
	id string
}

func (p *testProcess) ID() string {
	return p.id
}

func (p *testProcess) Start() error {
	return nil
}

func (p *testProcess) CloseStdin() error {
	return nil
}

func (p *testProcess) Resize(w, h int) error {
	return nil
}

func (p *testProcess) Stdio() runtime.Stdio {
	return runtime.Stdio{}
}

func (p *testProcess) SystemPid() int {
	return -1
}

func (p *testProcess) ExitFD() int {
	return -1
}

func (p *testProcess) ExitStatus() (uint32, error) {
	return runtime.UnknownStatus, nil
}

func (p *testProcess) Container() runtime.Container {
	return nil
}

func (p *testProcess) Spec() specs.ProcessSpec {
	return specs.ProcessSpec{}
}

func (p *testProcess) Signal(os.Signal) error {
	return nil
}

func (p *testProcess) Close() error {
	return nil
}

func (p *testProcess) State() runtime.State {
	return runtime.Running
}

func (p *testProcess) Wait() {
}

func TestSortProcesses(t *testing.T) {
	p := []runtime.Process{
		&testProcess{"ls"},
		&testProcess{"other"},
		&testProcess{"init"},
		&testProcess{"other2"},
	}
	s := &processSorter{p}
	sort.Sort(s)

	if id := p[len(p)-1].ID(); id != "init" {
		t.Fatalf("expected init but received %q", id)
	}
}
