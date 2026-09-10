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
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func main() {
	cmd := &cobra.Command{
		Short: "Compose Provider Example",
		Use:   "demo",
	}
	cmd.AddCommand(composeCommand())
	cmd.AddCommand(serveDemoCommand())
	err := cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	db   string
	size int
}

func composeCommand() *cobra.Command {
	c := &cobra.Command{
		Use:              "compose EVENT",
		TraverseChildren: true,
	}
	c.PersistentFlags().String("project-name", "", "compose project name") // unused

	var options options
	upCmd := &cobra.Command{
		Use: "up",
		Run: func(_ *cobra.Command, args []string) {
			up(options, args)
		},
		Args: cobra.ExactArgs(1),
	}

	upCmd.Flags().StringVar(&options.db, "type", "", "Database type (mysql, postgres, etc.)")
	_ = upCmd.MarkFlagRequired("type")
	upCmd.Flags().IntVar(&options.size, "size", 10, "Database size in GB")
	upCmd.Flags().String("name", "", "Name of the database to be created")
	_ = upCmd.MarkFlagRequired("name")

	downCmd := &cobra.Command{
		Use:  "down",
		Run:  down,
		Args: cobra.ExactArgs(1),
	}
	downCmd.Flags().String("name", "", "Name of the database to be deleted")
	_ = downCmd.MarkFlagRequired("name")

	stopCmd := &cobra.Command{
		Use:  "stop",
		Run:  stop,
		Args: cobra.ExactArgs(1),
	}

	c.AddCommand(upCmd, downCmd, stopCmd)
	c.AddCommand(metadataCommand(upCmd, downCmd, stopCmd))
	return c
}

// serveDemoCommand is the detached helper process behind the
// publish-endpoint demonstration: a TCP server on the given address
// answering every connection with a fixed HTTP response, exiting on its own
// after three minutes. It owns the port from bind to exit: the bound address
// is reported on stdout once listening, so the parent never has to probe or
// pre-reserve the port (no TOCTOU window, and works on Windows where handing
// a socket over ExtraFiles is not supported).
func serveDemoCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "serve-demo ADDR",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			listener, err := net.Listen("tcp", args[0])
			if err != nil {
				return err
			}
			fmt.Println(listener.Addr().String())
			go func() {
				time.Sleep(3 * time.Minute)
				os.Exit(0)
			}()
			for {
				conn, err := listener.Accept()
				if err != nil {
					return err
				}
				go func() {
					defer func() { _ = conn.Close() }()
					buf := make([]byte, 1024)
					_, _ = conn.Read(buf)
					_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 19\r\nConnection: close\r\n\r\nhello from provider"))
				}()
			}
		},
	}
}

const lineSeparator = "\n"

func up(options options, args []string) {
	servicename := args[0]
	fmt.Printf(`{ "type": "debug", "message": "Starting %s" }%s`, servicename, lineSeparator)

	// Ask the running Compose process for the resolved definition of the
	// service this provider manages. A Compose that predates the message
	// aborts on it, so only providers that require the configuration
	// should send it. The decoder must be created once and reused across
	// requests: it reads ahead, so a fresh decoder per request would discard
	// buffered bytes and hang on the next answer.
	responses := json.NewDecoder(os.Stdin)
	fmt.Printf(`{ "type": "get-service-config" }%s`, lineSeparator)
	var config struct {
		Provider struct {
			Type string `json:"type"`
		} `json:"provider"`
	}
	if err := responses.Decode(&config); err != nil {
		// error text is not JSON-safe either: encode, don't interpolate
		msg, _ := json.Marshal(map[string]string{"type": "error", "message": fmt.Sprintf("get-service-config failed: %v", err)})
		fmt.Println(string(msg))
		return
	}
	// values read from the configuration are not necessarily JSON-safe:
	// encode the message instead of interpolating it into a JSON literal
	setenv, _ := json.Marshal(map[string]string{"type": "setenv", "message": "CONFIG_TYPE=" + config.Provider.Type})
	fmt.Println(string(setenv))

	// When asked to, stand up a real endpoint on the host and publish it, so
	// compose deploys a relay and consumers reach it as http://<service>:80.
	if os.Getenv("PROVIDER_DEMO_ENDPOINT") != "" {
		// The subprocess binds the port itself and reports the resulting
		// address on its stdout; only then is the endpoint published. This
		// avoids the two races of a pre-reserved port: another process
		// grabbing it between release and re-bind, and publish-endpoint
		// pointing at a server that is not listening yet.
		// All interfaces, not loopback: on a plain Linux engine host-gateway
		// is the bridge IP, which cannot reach a host loopback bind.
		server := exec.Command(os.Args[0], "serve-demo", "0.0.0.0:0")
		stdout, err := server.StdoutPipe()
		if err != nil {
			fmt.Printf(`{ "type": "error", "message": "demo endpoint: %v" }%s`, err, lineSeparator)
			return
		}
		if err := server.Start(); err != nil {
			fmt.Printf(`{ "type": "error", "message": "demo endpoint: %v" }%s`, err, lineSeparator)
			return
		}
		// A crashed subprocess closes the pipe (EOF below); a hung one would
		// block the read forever, so kill it after a deadline — the read then
		// fails with EOF and lands on the same error path.
		watchdog := time.AfterFunc(30*time.Second, func() { _ = server.Process.Kill() })
		addr, err := bufio.NewReader(stdout).ReadString('\n')
		watchdog.Stop()
		if err != nil {
			fmt.Printf(`{ "type": "error", "message": "demo endpoint did not come up: %v" }%s`, err, lineSeparator)
			return
		}
		// the endpoint is announced as seen from THIS process's host —
		// the relay translates loopback into the container-visible name
		_, port, _ := net.SplitHostPort(strings.TrimSpace(addr))
		fmt.Printf(`{ "type": "publish-endpoint", "message": "80=localhost:%s" }%s`, port, lineSeparator)
	}

	for i := 0; i < options.size; i += 10 {
		time.Sleep(1 * time.Second)
		fmt.Printf(`{ "type": "info", "message": "Processing ... %d%%" }%s`, i*100/options.size, lineSeparator)
	}
	fmt.Printf(`{ "type": "setenv", "message": "URL=https://magic.cloud/%s" }%s`, servicename, lineSeparator)
	fmt.Printf(`{ "type": "rawsetenv", "message": "CLOUD_REGION=us-east-1" }%s`, lineSeparator)
}

func down(_ *cobra.Command, _ []string) {
	// A failing down can be simulated for tests and demos.
	if os.Getenv("PROVIDER_DOWN_FAILURE") != "" {
		fmt.Printf(`{ "type": "error", "message": "Permission error" }%s`, lineSeparator)
		return
	}
	fmt.Printf(`{ "type": "info", "message": "Resource removed" }%s`, lineSeparator)
}

func stop(_ *cobra.Command, _ []string) {
	if marker := os.Getenv("PROVIDER_STOP_MARKER"); marker != "" {
		_ = os.WriteFile(marker, []byte("stopped"), 0o600)
	}
}

func metadataCommand(upCmd, downCmd, stopCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use: "metadata",
		Run: func(cmd *cobra.Command, _ []string) {
			metadata(upCmd, downCmd, stopCmd)
		},
		Args: cobra.NoArgs,
	}
}

func metadata(upCmd, downCmd, stopCmd *cobra.Command) {
	metadata := ProviderMetadata{}
	metadata.Description = "Manage services on AwesomeCloud"
	metadata.Up = commandParameters(upCmd)
	metadata.Down = commandParameters(downCmd)
	stopParams := commandParameters(stopCmd)
	metadata.Stop = &stopParams
	jsonMetadata, err := json.Marshal(metadata)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(jsonMetadata))
}

func commandParameters(cmd *cobra.Command) CommandMetadata {
	cmdMetadata := CommandMetadata{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_, isRequired := f.Annotations[cobra.BashCompOneRequiredFlag]
		cmdMetadata.Parameters = append(cmdMetadata.Parameters, Metadata{
			Name:        f.Name,
			Description: f.Usage,
			Required:    isRequired,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
		})
	})
	return cmdMetadata
}

type ProviderMetadata struct {
	Description string           `json:"description"`
	Up          CommandMetadata  `json:"up"`
	Down        CommandMetadata  `json:"down"`
	Stop        *CommandMetadata `json:"stop,omitempty"`
}

type CommandMetadata struct {
	Parameters []Metadata `json:"parameters"`
}

type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}
