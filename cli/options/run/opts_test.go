package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/containers"
)

type RunOptsSuite struct {
	suite.Suite
}

func (s *RunOptsSuite) TestPortParse() {
	testCases := []struct {
		in       string
		expected []containers.Port
	}{
		{
			in: "80",
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
			},
		},
		{
			in: "80:80",
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
			},
		},
		{
			in: "80:80/udp",
			expected: []containers.Port{
				{
					ContainerPort: 80,
					HostPort:      80,
					Protocol:      "udp",
				},
			},
		},
		{
			in: "8080:80",
			expected: []containers.Port{
				{
					HostPort:      8080,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
			},
		},
		{
			in: "192.168.0.2:8080:80",
			expected: []containers.Port{
				{
					HostPort:      8080,
					ContainerPort: 80,
					Protocol:      "tcp",
					HostIP:        "192.168.0.2",
				},
			},
		},
		{
			in: "80-81:80-81",
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
				{
					HostPort:      81,
					ContainerPort: 81,
					Protocol:      "tcp",
				},
			},
		},
	}

	for _, testCase := range testCases {
		opts := Opts{
			Publish: []string{testCase.in},
		}
		result, err := opts.toPorts()
		require.Nil(s.T(), err)
		assert.ElementsMatch(s.T(), testCase.expected, result)
	}
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(RunOptsSuite))
}
