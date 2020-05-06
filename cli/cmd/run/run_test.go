package run

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// AzureNameRegex is used to validate container names
	// Regex was taken from server side error:
	// The container name must contain no more than 63 characters and must match the regex '[a-z0-9]([-a-z0-9]*[a-z0-9])?' (e.g. 'my-name').
	AzureNameRegex = regexp.MustCompile("[a-z0-9]([-a-z0-9]*[a-z0-9])")
)

// TestAzureRandomName ensures compliance with Azure naming requirements
func TestAzureRandomName(t *testing.T) {
	n := getRandomName()
	require.Less(t, len(n), 64)
	require.Greater(t, len(n), 1)
	require.Regexp(t, AzureNameRegex, n)
}
