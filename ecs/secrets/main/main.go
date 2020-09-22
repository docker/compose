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

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/docker/compose-cli/ecs/secrets"
)

const secretsFolder = "/run/secrets"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprint(os.Stderr, "usage: secrets <json encoded []Secret>")
		os.Exit(1)
	}

	var input []secrets.Secret
	err := json.Unmarshal([]byte(os.Args[1]), &input)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}

	for _, secret := range input {
		err := secrets.CreateSecretFiles(secret, secretsFolder)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
}
