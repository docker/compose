package utils

import (
	"os"
	"strings"
)

func Environment() map[string]string {
	vars := make(map[string]string)
	env := os.Environ()
	for _, v := range env {
		k := strings.SplitN(v, "=", 2)
		vars[k[0]] = k[1]
	}
	return vars
}
