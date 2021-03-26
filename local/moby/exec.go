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

package moby

import (
	"os"
	"os/exec"
	"os/signal"

	"github.com/docker/compose-cli/cli/mobycli/resolvepath"
)

// ComDockerCli name of the classic cli binary
const ComDockerCli = "com.docker.cli"

// Exec delegates to com.docker.cli
func Exec(args []string) error {
	// look up the path of the classic cli binary
	execBinary, err := resolvepath.LookPath(ComDockerCli)
	if err != nil {
		return err
	}
	cmd := exec.Command(execBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	signals := make(chan os.Signal, 1)
	childExit := make(chan bool)
	signal.Notify(signals) // catch all signals
	go func() {
		for {
			select {
			case sig := <-signals:
				if cmd.Process == nil {
					continue // can happen if receiving signal before the process is actually started
				}
				// nolint errcheck
				cmd.Process.Signal(sig)
			case <-childExit:
				return
			}
		}
	}()
	err = cmd.Run()
	childExit <- true
	return err
}
