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
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/docker/api/client"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var exampleCommand = cli.Command{
	Name:  "example",
	Usage: "sample command using backend, to be removed later",
	Action: func(clix *cli.Context) error {
		// return information for the current context
		ctx, cancel := client.NewContext()
		defer cancel()

		// get our current context
		ctx = current(ctx)

		client, err := connect(ctx)
		if err != nil {
			return errors.Wrap(err, "cannot connect to backend")
		}
		defer client.Close()

		info, err := client.BackendInformation(ctx, &types.Empty{})
		if err != nil {
			return errors.Wrap(err, "fetch backend information")
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		return enc.Encode(info)
	},
}

// mock information for getting context
// factor out this into a context store package
func current(ctx context.Context) context.Context {
	// test backend address
	return context.WithValue(ctx, backendAddressKey{}, "/tmp/backend.sock")
}

func connect(ctx context.Context) (*client.Client, error) {
	address, err := BackendAddress(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "no backend address")
	}
	c, err := client.New("unix://"+address, 500*time.Millisecond)
	if err != nil {
		if err != context.DeadlineExceeded {
			return nil, errors.Wrap(err, "connect to backend")
		}
		// the backend is not running so start it
		cmd := exec.Command("backend-example", "--address", address)
		go cmd.Wait()

		if err := cmd.Start(); err != nil {
			return nil, errors.Wrap(err, "start backend")
		}
		cl, e := client.New("unix://"+address, 10*time.Second)
		return cl, e
	}
	return c, nil
}

type backendAddressKey struct{}

func BackendAddress(ctx context.Context) (string, error) {
	v, ok := ctx.Value(backendAddressKey{}).(string)
	if !ok {
		return "", errors.New("no backend address key")
	}
	return v, nil
}
