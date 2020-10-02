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
	"bytes"
	"encoding/json"
)

const standardIndentation = "    "

// ToStandardJSON return a string with the JSON representation of the interface{}
func ToStandardJSON(i interface{}) (string, error) {
	return ToJSON(i, "", standardIndentation)
}

// ToJSON return a string with the JSON representation of the interface{}
func ToJSON(i interface{}, prefix string, indentation string) (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent(prefix, indentation)
	err := encoder.Encode(i)
	return buffer.String(), err
}
