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
	"strconv"

	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/pkg/api"
)

func networkFilter(name string) string {
	return naming.LabelFilter(api.NetworkLabel, name)
}

func oneOffFilter(b bool) string {
	v := "False"
	if b {
		v = "True"
	}
	return naming.LabelFilter(api.OneoffLabel, v)
}

func containerNumberFilter(index int) string {
	return naming.LabelFilter(api.ContainerNumberLabel, strconv.Itoa(index))
}

// hookFilter returns a label filter that matches containers by hook type
// (stored in api.HookLabel). hookType is parametric for future hook types
// (e.g. post_start); today all callers pass preStartHookType.
//
//nolint:unparam
func hookFilter(hookType string) string {
	return naming.LabelFilter(api.HookLabel, hookType)
}
