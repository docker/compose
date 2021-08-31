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
	"strings"

	"github.com/hashicorp/go-multierror"
)

// SetMultiErrorFormat set cli default format for multi-errors
func SetMultiErrorFormat(errs *multierror.Error) {
	if errs != nil {
		errs.ErrorFormat = formatErrors
	}
}

func formatErrors(errs []error) string {
	messages := make([]string, len(errs))
	for i, err := range errs {
		messages[i] = "Error: " + err.Error()
	}
	return strings.Join(messages, "\n")
}
