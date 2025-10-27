/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package progress

import (
	"context"
)

type progressFunc func(context.Context) error

func Run(ctx context.Context, pf progressFunc, operation string, bus EventProcessor) error {
	bus.Start(ctx, operation)
	err := pf(ctx)
	bus.Done(operation, err != nil)
	return err
}

const (
	// ModeAuto detect console capabilities
	ModeAuto = "auto"
	// ModeTTY use terminal capability for advanced rendering
	ModeTTY = "tty"
	// ModePlain dump raw events to output
	ModePlain = "plain"
	// ModeQuiet don't display events
	ModeQuiet = "quiet"
	// ModeJSON outputs a machine-readable JSON stream
	ModeJSON = "json"
)

// Mode define how progress should be rendered, either as ModePlain or ModeTTY
var Mode = ModeAuto
