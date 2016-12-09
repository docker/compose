package execution

import (
	"context"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type CreateOpts struct {
	Bundle  string
	Console bool
	Stdin   string
	Stdout  string
	Stderr  string
}

type StartProcessOpts struct {
	Spec    specs.Process
	Console bool
	Stdin   string
	Stdout  string
	Stderr  string
}

type Executor interface {
	Create(ctx context.Context, id string, o CreateOpts) (*Container, error)
	Pause(context.Context, *Container) error
	Resume(context.Context, *Container) error
	Status(context.Context, *Container) (Status, error)
	List(context.Context) ([]*Container, error)
	Load(ctx context.Context, id string) (*Container, error)
	Delete(context.Context, *Container) error
	Start(context.Context, *Container) error

	StartProcess(context.Context, *Container, StartProcessOpts) (Process, error)
	SignalProcess(ctx context.Context, c *Container, id string, sig os.Signal) error
	DeleteProcess(ctx context.Context, c *Container, id string) error
}
