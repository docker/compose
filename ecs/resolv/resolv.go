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

package resolv

import (
	"os"
	"strings"
)

// SetSearchDomains appends a `search` directive to resolv.conf file for domains
func SetSearchDomains(file string, domains ...string) error {
	search := strings.Join(domains, " ")

	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = f.WriteString("\nsearch " + search)
	return err
}
