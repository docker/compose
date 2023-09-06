package cmdtrace

import (
	flag "github.com/spf13/pflag"
	"reflect"
	"testing"
)

func TestGetFlags(t *testing.T) {
	// Initialize flagSet with flags
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	var (
		detach  string
		timeout string
	)
	fs.StringVar(&detach, "detach", "d", "")
	fs.StringVar(&timeout, "timeout", "t", "")
	_ = fs.Set("detach", "detach")
	_ = fs.Set("timeout", "timeout")

	tests := []struct {
		name     string
		input    *flag.FlagSet
		expected []string
	}{
		{
			name:     "NoFlags",
			input:    flag.NewFlagSet("NoFlags", flag.ContinueOnError),
			expected: nil,
		},
		{
			name:     "Flags",
			input:    fs,
			expected: []string{"detach", "timeout"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getFlags(test.input)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Expected %v, but got %v", test.expected, result)
			}
		})
	}

}
