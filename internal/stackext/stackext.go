/*
   Copyright 2026 Docker Compose CLI authors

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

// Package stackext is a proof of concept for publishing the resolved Compose
// project to the engine through the com.docker.compose.stack.v0 extension
// point of the moby extensions framework (moby/moby#53021): a gRPC service
// exposed by the daemon on the API socket itself (h2c). Best effort by
// design: an engine without the extension answers codes.Unimplemented, an
// engine without h2c support fails at transport level, and both are treated
// as "feature absent".
package stackext

import (
	"context"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	cligrpc "github.com/docker/cli/cli/grpc"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

const publishMethod = "/com.docker.compose.stack.v0.Stack/Publish"

// rawCodec sends pre-encoded protobuf wire bytes as-is, mirroring the
// passthrough codec the extensions gRPC proxy uses daemon-side. It reports
// "proto" so the request carries the default content-subtype.
type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error)    { return v.([]byte), nil }
func (rawCodec) Unmarshal(data []byte, v any) error { *(v.(*[]byte)) = data; return nil }
func (rawCodec) Name() string                     { return "proto" }

// Publish pushes the resolved project definition to the engine's compose
// stack extension point, when the engine exposes one. Never fails the caller.
// The connection goes through cli/grpc.Connect (docker/cli#7183), which hides
// the transport of the current context (unix/npipe socket, tcp with or
// without TLS, connection helpers) and the h2c/legacy-upgrade negotiation.
func Publish(ctx context.Context, dockerCli cligrpc.DockerCLI, project *types.Project, complete bool) {
	model, err := project.MarshalYAML()
	if err != nil {
		logrus.Debugf("stackext: cannot marshal project: %v", err)
		return
	}

	// hand-encoded com.docker.compose.stack.v0.PublishRequest
	var payload []byte
	payload = protowire.AppendTag(payload, 1, protowire.BytesType)
	payload = protowire.AppendString(payload, project.Name)
	payload = protowire.AppendTag(payload, 2, protowire.BytesType)
	payload = protowire.AppendBytes(payload, model)
	if complete {
		payload = protowire.AppendTag(payload, 3, protowire.VarintType)
		payload = protowire.AppendVarint(payload, 1)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := cligrpc.Connect(ctx, dockerCli)
	if err != nil {
		logrus.Debugf("stackext: cannot connect to the engine gRPC endpoint: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	var reply []byte
	err = conn.Invoke(ctx, publishMethod, payload, &reply, grpc.ForceCodec(rawCodec{}))
	switch status.Code(err) {
	case codes.OK:
		logrus.Infof("stackext: compose stack definition published to engine extension")
	case codes.Unimplemented:
		logrus.Debugf("stackext: engine does not expose the compose stack extension point")
	default:
		logrus.Debugf("stackext: publish failed: %v", err)
	}
}
