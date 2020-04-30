/*
	Copyright (c) 2020 Docker Inc.

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

package cmd

import (
	"context"
	"encoding/json"
	"os"

	"github.com/docker/api/client"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var ExampleCommand = cobra.Command{
	Use:   "example",
	Short: "sample command using backend, to be removed later",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		c, err := client.New(ctx)
		if err != nil {
			return errors.Wrap(err, "cannot connect to backend")
		}

		info, err := c.BackendInformation(ctx, &empty.Empty{})
		if err != nil {
			return errors.Wrap(err, "fetch backend information")
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		return enc.Encode(info)
	},
}

type backendAddressKey struct{}

func BackendAddress(ctx context.Context) (string, error) {
	v, ok := ctx.Value(backendAddressKey{}).(string)
	if !ok {
		return "", errors.New("no backend address key")
	}
	return v, nil
}
