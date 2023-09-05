package cmdtrace

import (
	"reflect"
	"testing"
)

func TestFilterForFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "NoFlags",
			input:    []string{"compose", "up"},
			expected: nil,
		},
		{
			name:     "Flags",
			input:    []string{"compose", "up", "-d", "--timeout", "100"},
			expected: []string{"-d", "--timeout"},
		},
		{
			name:     "Empty",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "SpecialCharacters",
			input:    []string{"--timeout100", "-d**", "123456789&^%$#@!"},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := filterForFlags(test.input)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Expected %v, but got %v", test.expected, result)
			}
		})
	}

}
