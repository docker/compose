/*
	Copyright (c) 2019 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/docker/api/context"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func init() {
	// initial hack to get the path of the project's bin dir
	// into the env of this cli for development

	path, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("PATH", fmt.Sprintf("$PATH:%s", path)); err != nil {
		panic(err)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "docker"
	app.Usage = "Docker for the 2020s"
	app.UseShortOptionHandling = true
	app.EnableBashCompletion = true
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in the logs",
		},
		context.ConfigFlag,
		context.ContextFlag,
	}

	// Make a copy of the default HelpPrinter function
	originalHelpPrinter := cli.HelpPrinter
	// Change the HelpPrinter function to shell out to the Moby CLI help
	// when the current context is pointing to Docker engine
	// else we use the copy of the original HelpPrinter
	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		ctx, err := context.GetContext()
		if err != nil {
			logrus.Fatal(err)
		}
		if ctx.Metadata.Type == "Moby" {
			shellOutToDefaultEngine()
		} else {
			originalHelpPrinter(w, templ, data)
		}
	}

	app.Before = func(clix *cli.Context) error {
		if clix.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		ctx, err := context.GetContext()
		if err != nil {
			logrus.Fatal(err)
		}
		if ctx.Metadata.Type == "Moby" {
			shellOutToDefaultEngine()
		}
		// TODO select backend based on context.Metadata.Type
		return nil
	}
	app.Commands = []cli.Command{
		contextCommand,
		exampleCommand,
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func shellOutToDefaultEngine() {
	cmd := exec.Command("/Applications/Docker.app/Contents/Resources/bin/docker", os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				os.Exit(exiterr.ExitCode())
			}
			os.Exit(1)
		}
	}
	os.Exit(0)
}
