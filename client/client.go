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

package client

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "github.com/docker/api/backend/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// NewContext is a context that is canceled when a signal is
// sent to the process
func NewContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s := make(chan os.Signal)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-s
		cancel()
	}()
	return ctx, cancel
}

// New returns a GRPC client
func New(address string, timeout time.Duration) (*Client, error) {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = 3 * time.Second
	backoffConfig.BaseDelay = 10 * time.Millisecond
	connParams := grpc.ConnectParams{
		Backoff: backoffConfig,
	}
	opts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithConnectParams(connParams),
		grpc.WithBlock(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:          conn,
		BackendClient: v1.NewBackendClient(conn),
	}, nil
}

type Client struct {
	conn *grpc.ClientConn
	v1.BackendClient
}

func (c *Client) Close() error {
	return c.conn.Close()
}
