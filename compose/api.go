package compose

import (
	"context"
)

// Service manages a compose project
type Service interface {
	// Up executes the equivalent to a `compose up`
	Up(ctx context.Context, opts ProjectOptions) error
	// Down executes the equivalent to a `compose down`
	Down(ctx context.Context, opts ProjectOptions) error
}
