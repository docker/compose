package supervisor

import (
	"os"
	"sort"
	"testing"

	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/specs"
)

type testProcess struct {
	id string
}

func (p *testProcess) ID() string {
	return p.id
}

func (p *testProcess) Stdin() string {
	return ""
}

func (p *testProcess) Stdout() string {
	return ""
}

func (p *testProcess) Stderr() string {
	return ""
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

func (p *testProcess) Spec() specs.Process {
	return specs.Process{}
}

func (p *testProcess) Signal(os.Signal) error {
	return nil
}

func (p *testProcess) Close() error {
	return nil
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
