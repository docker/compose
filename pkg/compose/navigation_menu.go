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

package compose

import (
	"context"
	"os"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/docker/compose/v5/pkg/api"
)

// NavigationMenu is the interactive keyboard menu of a foreground `compose up`.
// The terminal rendering is up to the implementation, which the CLI provides
// (see WithNavigationMenu): the library only drives it.
type NavigationMenu interface {
	// Decorate returns a LogConsumer that keeps the menu displayed below the logs
	Decorate(api.LogConsumer) api.LogConsumer
	// EnableWatch makes the menu able to toggle watch, which is initially enabled or not
	EnableWatch(enabled bool, watcher api.Feature)
	// EnableDetach sets the function run when the user asks to detach
	EnableDetach(detach func())
	// Open takes control of the keyboard. Keys typed from then on are kept until
	// Run consumes them. An error means the menu can't be used (no terminal, …)
	// and the session carries on without it.
	Open() error
	// Run reacts to the keys typed by the user until ctx is done. It is called
	// once, after Open and after the menu has been fully configured.
	Run(ctx context.Context, project *types.Project, options api.UpOptions)
	// Close gives the keyboard back. It can be called several times, and also
	// when Open failed.
	Close() error
}

// NavigationMenuFactory creates the NavigationMenu of an `up` session. It must
// return a nil interface, not a typed nil pointer, when there is no menu.
// isDockerDesktopActive and isLogsViewEnabled tell which Docker Desktop
// integrations the menu can offer, and signals is the channel the menu notifies
// with an interruption when the user asks to stop the application.
type NavigationMenuFactory func(isDockerDesktopActive, isLogsViewEnabled bool, signals chan<- os.Signal) NavigationMenu

// WithNavigationMenu configures the interactive menu displayed by a foreground
// `up` when api.StartOptions.NavigationMenu is set. Without it, no menu is
// displayed.
func WithNavigationMenu(factory NavigationMenuFactory) Option {
	return func(s *composeService) error {
		s.navigationMenu = factory
		return nil
	}
}
