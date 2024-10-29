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
	"fmt"
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/store"
	"github.com/docker/compose/v2/internal/memnet"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const otelConfigFieldName = "otel"

// traceClientFromDockerContext creates a gRPC OTLP client based on metadata
// from the active Docker CLI context.
func traceClientFromDockerContext(dockerCli command.Cli, otelEnv envMap) (otlptrace.Client, error) {
	// attempt to extract an OTEL config from the Docker context to enable
	// automatic integration with Docker Desktop;
	cfg, err := ConfigFromDockerContext(dockerCli.ContextStore(), dockerCli.CurrentContext())
	if err != nil {
		return nil, fmt.Errorf("loading otel config from docker context metadata: %w", err)
	}

	if cfg.Endpoint == "" {
		return nil, nil
	}

	// HACK: unfortunately _all_ public OTEL initialization functions
	// 	implicitly read from the OS env, so temporarily unset them all and
	// 	restore afterwards
	defer func() {
		for k, v := range otelEnv {
			if err := os.Setenv(k, v); err != nil {
				panic(fmt.Errorf("restoring env for %q: %w", k, err))
			}
		}
	}()
	for k := range otelEnv {
		if err := os.Unsetenv(k); err != nil {
			return nil, fmt.Errorf("stashing env for %q: %w", k, err)
		}
	}

	conn, err := grpc.NewClient(cfg.Endpoint,
		grpc.WithContextDialer(memnet.DialEndpoint),
		// this dial is restricted to using a local Unix socket / named pipe,
		// so there is no need for TLS
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing otel connection from docker context metadata: %w", err)
	}

	client := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(conn))
	return client, nil
}

// ConfigFromDockerContext inspects extra metadata included as part of the
// specified Docker context to try and extract a valid OTLP client configuration.
func ConfigFromDockerContext(st store.Store, name string) (OTLPConfig, error) {
	meta, err := st.GetMetadata(name)
	if err != nil {
		return OTLPConfig{}, err
	}

	var otelCfg interface{}
	switch m := meta.Metadata.(type) {
	case command.DockerContext:
		otelCfg = m.AdditionalFields[otelConfigFieldName]
	case map[string]interface{}:
		otelCfg = m[otelConfigFieldName]
	}
	if otelCfg == nil {
		return OTLPConfig{}, nil
	}

	otelMap, ok := otelCfg.(map[string]interface{})
	if !ok {
		return OTLPConfig{}, fmt.Errorf(
			"unexpected type for field %q: %T (expected: %T)",
			otelConfigFieldName,
			otelCfg,
			otelMap,
		)
	}

	// keys from https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/
	cfg := OTLPConfig{
		Endpoint: valueOrDefault[string](otelMap, "OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
	return cfg, nil
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
