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
	"testing"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/store"
	"github.com/stretchr/testify/require"
)

var testStoreCfg = store.NewConfig(
	func() interface{} {
		return &map[string]interface{}{}
	},
)

func TestCreateResource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// this can fail if semconv gets out of sync
	res, err := createResource(ctx)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestExtractOtelFromContext(t *testing.T) {
	if testing.Short() {
		t.Skip("Requires filesystem access")
	}

	dir := t.TempDir()

	st := store.New(dir, testStoreCfg)
	err := st.CreateOrUpdate(store.Metadata{
		Name: "test",
		Metadata: command.DockerContext{
			Description: t.Name(),
			AdditionalFields: map[string]interface{}{
				"otel": map[string]interface{}{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:1234",
				},
			},
		},
		Endpoints: make(map[string]interface{}),
	})
	require.NoError(t, err)

	cfg, err := ConfigFromDockerContext(st, "test")
	require.NoError(t, err)
	require.Equal(t, "localhost:1234", cfg.Endpoint)
}
