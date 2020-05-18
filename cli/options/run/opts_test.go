package run

import (
	"errors"
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

func (s *RunOptsSuite) TestLabels() {
	testCases := []struct {
		in            []string
		expected      map[string]string
		expectedError error
	}{
		{
			in: []string{"label=value"},
			expected: map[string]string{
				"label": "value",
			},
			expectedError: nil,
		},
		{
			in: []string{"label=value", "label=value2"},
			expected: map[string]string{
				"label": "value2",
			},
			expectedError: nil,
		},
		{
			in: []string{"label=value", "label2=value2"},
			expected: map[string]string{
				"label":  "value",
				"label2": "value2",
			},
			expectedError: nil,
		},
		{
			in:            []string{"label"},
			expected:      nil,
			expectedError: errors.New(`wrong label format "label"`),
		},
	}

	for _, testCase := range testCases {
		result, err := toLabels(testCase.in)
		assert.Equal(s.T(), testCase.expectedError, err)
		assert.Equal(s.T(), testCase.expected, result)
	}
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(RunOptsSuite))
}
