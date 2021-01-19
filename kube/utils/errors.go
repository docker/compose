// +build kube

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

package utils

import (
	"fmt"
	"strings"
)

func CombineErrors(errors []error) error {
	if len(errors) == 0 {
		return nil
	}
	if len(errors) == 1 {
		return errors[0]
	}
	err := combinedError{}
	for _, e := range errors {
		if c, ok := e.(combinedError); ok {
			err.errors = append(err.errors, c.errors...)
		} else {
			err.errors = append(err.errors, e)
		}
	}
	return combinedError{errors}
}

type combinedError struct {
	errors []error
}

func (c combinedError) Error() string {
	points := make([]string, len(c.errors))
	for i, err := range c.errors {
		points[i] = fmt.Sprintf("* %s", err.Error())
	}
	return fmt.Sprintf(
		"%d errors occurred:\n\t%s",
		len(c.errors), strings.Join(points, "\n\t"))
}
