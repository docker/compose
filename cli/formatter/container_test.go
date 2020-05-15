package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"

	"github.com/docker/api/cli/options/run"
)

func TestDisplayPorts(t *testing.T) {
	testCases := []struct {
		name     string
		in       []string
		expected string
	}{
		{
			name:     "simple",
			in:       []string{"80"},
			expected: "0.0.0.0:80->80/tcp",
		},
		{
			name:     "different ports",
			in:       []string{"80:90"},
			expected: "0.0.0.0:80->90/tcp",
		},
		{
			name:     "host ip",
			in:       []string{"192.168.0.1:80:90"},
			expected: "192.168.0.1:80->90/tcp",
		},
		{
			name:     "port range",
			in:       []string{"80-90:80-90"},
			expected: "0.0.0.0:80-90->80-90/tcp",
		},
		{
			name:     "grouping",
			in:       []string{"80:80", "81:81"},
			expected: "0.0.0.0:80-81->80-81/tcp",
		},
		{
			name:     "groups",
			in:       []string{"80:80", "82:82"},
			expected: "0.0.0.0:80->80/tcp, 0.0.0.0:82->82/tcp",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runOpts := run.Opts{
				Publish: testCase.in,
			}
			containerConfig, err := runOpts.ToContainerConfig("test")
			require.Nil(t, err)

			out := PortsString(containerConfig.Ports)
			assert.Equal(t, testCase.expected, out)
		})
	}
}
