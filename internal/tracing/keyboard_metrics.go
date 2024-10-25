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

package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

func KeyboardMetrics(ctx context.Context, enabled, isDockerDesktopActive, isWatchConfigured bool) {
	commandAvailable := []string{}
	if isDockerDesktopActive {
		commandAvailable = append(commandAvailable, "gui")
		commandAvailable = append(commandAvailable, "gui/composeview")
	}
	if isWatchConfigured {
		commandAvailable = append(commandAvailable, "watch")
	}

	AddAttributeToSpan(ctx,
		attribute.Bool("navmenu.enabled", enabled),
		attribute.StringSlice("navmenu.command_available", commandAvailable))
}
