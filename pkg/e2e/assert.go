package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequireServiceState ensures that the container is in the expected state
// (running or exited).
func RequireServiceState(t testing.TB, cli *CLI, service string, state string) {
	t.Helper()
	psRes := cli.RunDockerComposeCmd(t, "ps", "--format=json", service)
	var psOut []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(psRes.Stdout()), &psOut),
		"Invalid `compose ps` JSON output")

	for _, svc := range psOut {
		require.Equal(t, service, svc["Service"],
			"Found ps output for unexpected service")
		require.Equalf(t,
			strings.ToLower(state),
			strings.ToLower(svc["State"].(string)),
			"Service %q (%s) not in expected state",
			service, svc["Name"],
		)
	}
}
