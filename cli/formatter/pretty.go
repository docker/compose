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

package formatter

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// PrintPrettySection prints a tabbed section on the writer parameter
func PrintPrettySection(out io.Writer, printer func(writer io.Writer), headers ...string) error {
	w := tabwriter.NewWriter(out, 20, 1, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(headers, "\t"))
	printer(w)
	return w.Flush()
}
