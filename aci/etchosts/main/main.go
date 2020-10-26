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
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/compose-cli/aci/etchosts"
)

const hosts = "/etc/hosts"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, "usage: hosts HOSTNAME [HOSTNAME]")
		os.Exit(1)
	}

	err := etchosts.SetHostNames(hosts, os.Args[1:]...)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}

	// ACI restart policy is currently at container group level, cannot let the sidecar terminate quietly once /etc/hosts has been edited
	// Pause forever (until someone explicitly terminates this process ; go is not happy to stop all goroutines otherwise)
	exitSignal := make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGINT, syscall.SIGTERM)
	<-exitSignal
}
