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
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

// profilesOnlyServices resolves the service names targeted by the --profiles-only
// flag: services enabled by one of the active profiles, or services belonging to
// any profile when no profile is active. It returns the project to run the
// command with, which has all profiles enabled when none was active. An empty
// service list with a nil error means there is nothing to do.
func profilesOnlyServices(project *types.Project, services []string, action string, w io.Writer) (*types.Project, []string, error) {
	if len(services) > 0 {
		return nil, nil, errors.New("--profiles-only cannot be combined with service names, naming a service already activates its profiles")
	}
	if project == nil {
		return nil, nil, errors.New("--profiles-only requires the project's compose file(s), pass --file or run the command from the project directory")
	}

	// COMPOSE_PROFILES being unset yields a single blank profile name, filter
	// such entries out so it is treated as "no active profile"
	activeProfiles := slices.DeleteFunc(slices.Clone(project.Profiles), func(p string) bool {
		return p == ""
	})
	if len(activeProfiles) == 0 {
		var err error
		project, err = project.WithProfiles([]string{"*"})
		if err != nil {
			return nil, nil, err
		}
	}

	var names, allProfiles []string
	for name, service := range project.Services {
		if len(service.Profiles) == 0 {
			continue
		}
		names = append(names, name)
		allProfiles = append(allProfiles, service.Profiles...)
	}
	slices.Sort(names)
	slices.Sort(allProfiles)
	allProfiles = slices.Compact(allProfiles)

	switch {
	case len(names) == 0 && len(activeProfiles) > 0:
		_, _ = fmt.Fprintf(w, "no services matched the active profiles [%s]\n", strings.Join(activeProfiles, " "))
	case len(names) == 0:
		_, _ = fmt.Fprintln(w, "no service in this project uses profiles")
	case len(activeProfiles) > 0:
		_, _ = fmt.Fprintf(w, "%s services in profiles [%s]\n", action, strings.Join(activeProfiles, " "))
	default:
		_, _ = fmt.Fprintf(w, "%s services from all profiles [%s] as none is active\n", action, strings.Join(allProfiles, " "))
	}
	return project, names, nil
}
