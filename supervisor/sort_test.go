package supervisor

import (
	"os"
	"sort"
	"testing"

	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/specs"
)

type testProcess struct {
	id string
}

func (p *testProcess) ID() string {
	return p.id
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

func (p *testProcess) ExitStatus() (int, error) {
	return -1, nil
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
