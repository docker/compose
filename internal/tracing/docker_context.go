/*
   Copyright 2023 Docker Compose CLI authors

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

package tracing

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/store"
	"github.com/docker/compose/v2/internal/memnet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const otelConfigFieldName = "otel"

// DockerOTLPConfig contains the necessary values to initialize an OTLP client
// manually.
//
// This supports a minimal set of options based on what is necessary for
// automatic OTEL configuration from Docker context metadata.
type DockerOTLPConfig struct {
	Endpoint string
}

// ConfigFromDockerContext inspects extra metadata included as part of the
// specified Docker context to try and extract a valid OTLP client configuration.
func ConfigFromDockerContext(st store.Store, name string) (DockerOTLPConfig, error) {
	meta, err := st.GetMetadata(name)
	if err != nil {
		return DockerOTLPConfig{}, err
	}

	var otelCfg interface{}
	switch m := meta.Metadata.(type) {
	case command.DockerContext:
		otelCfg = m.AdditionalFields[otelConfigFieldName]
	case map[string]interface{}:
		otelCfg = m[otelConfigFieldName]
	}
	if otelCfg == nil {
		return DockerOTLPConfig{}, nil
	}

	otelMap, ok := otelCfg.(map[string]interface{})
	if !ok {
		return DockerOTLPConfig{}, fmt.Errorf(
			"unexpected type for field %q: %T (expected: %T)",
			otelConfigFieldName,
			otelCfg,
			otelMap,
		)
	}

	// keys from https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/
	cfg := DockerOTLPConfig{
		Endpoint: valueOrDefault[string](otelMap, "OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
	return cfg, nil
}

// grpcConnection creates an OTLP/gRPC connection based on the Docker context configuration.
//
// If no endpoint is defined in the config, nil is returned.
func grpcConnection(ctx context.Context, cfg DockerOTLPConfig) (*grpc.ClientConn, error) {
	if cfg.Endpoint == "" {
		return nil, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	conn, err := grpc.DialContext(
		dialCtx,
		cfg.Endpoint,
		grpc.WithContextDialer(memnet.DialEndpoint),
		// this dial is restricted to using a local Unix socket / named pipe,
		// so there is no need for TLS
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dialing Docker otel: %w", err)
	}
	return conn, nil
}

// valueOrDefault returns the type-cast value at the specified key in the map
// if present and the correct type; otherwise, it returns the default value for
// T.
func valueOrDefault[T any](m map[string]interface{}, key string) T {
	if v, ok := m[key].(T); ok {
		return v
	}
	return *new(T)
}

// withoutOTelEnv runs a function while temporarily "hiding" all OTEL_ prefixed
// env vars and restoring them afterward.
//
// Unfortunately, the public OTEL exporter constructors ALWAYS implicitly read
// from the OS env, so this is necessary to allow for custom client construction
// without interference.
func withoutOTelEnv[T any](otelEnv envMap, fn func() (T, error)) (T, error) {
	for k := range otelEnv {
		if err := os.Unsetenv(k); err != nil {
			panic(fmt.Errorf("stashing env for %q: %w", k, err))
		}
	}

	defer func() {
		for k, v := range otelEnv {
			if err := os.Setenv(k, v); err != nil {
				panic(fmt.Errorf("restoring env for %q: %w", k, err))
			}
		}
	}()
	return fn()
}
