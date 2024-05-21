/*
   Copyright 2024 Docker Compose CLI authors

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

package experimental

import (
	"context"
	"os"
	"strconv"

	"github.com/docker/compose/v2/internal/desktop"
)

// envComposeExperimentalGlobal can be set to a falsy value (e.g. 0, false) to
// globally opt-out of any experimental features in Compose.
const envComposeExperimentalGlobal = "COMPOSE_EXPERIMENTAL"

// State of experiments (enabled/disabled) based on environment and local config.
type State struct {
	// active is false if experiments have been opted-out of globally.
	active        bool
	desktopValues desktop.FeatureFlagResponse
}

func NewState() *State {
	// experimental features have individual controls, but users can opt out
	// of ALL experiments easily if desired
	experimentsActive := true
	if v := os.Getenv(envComposeExperimentalGlobal); v != "" {
		experimentsActive, _ = strconv.ParseBool(v)
	}
	return &State{
		active: experimentsActive,
	}
}

func (s *State) Load(ctx context.Context, client *desktop.Client) error {
	if !s.active {
		// user opted out of experiments globally, no need to load state from
		// Desktop
		return nil
	}

	if client == nil {
		// not running under Docker Desktop
		return nil
	}

	desktopValues, err := client.FeatureFlags(ctx)
	if err != nil {
		return err
	}
	s.desktopValues = desktopValues
	return nil
}

func (s *State) NavBar() bool {
	return s.determineFeatureState("ComposeNav")
}

func (s *State) ComposeUI() bool {
	return s.determineFeatureState("ComposeUIView")
}

func (s *State) determineFeatureState(name string) bool {
	if s == nil || !s.active || s.desktopValues == nil {
		return false
	}
	// TODO(milas): we should add individual environment variable overrides
	// 	per-experiment in a generic way here
	return s.desktopValues[name].Enabled
}
