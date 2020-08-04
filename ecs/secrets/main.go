package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// return codes:
// 1: failed to read secret from env
// 2: failed to parse hierarchical secret
// 3: failed to write secret content into file
func main() {
	for _, name := range os.Args[1:] {
		i := strings.Index(name, ":")
		var keys []string
		if i > 0 {
			keys = strings.Split(name[i+1:], ",")
			name = name[:i]
		}
		value, ok := os.LookupEnv(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "%q variable not set", name)
			os.Exit(1)
		}

		secrets := filepath.Join("/run/secrets", name)

		if len(keys) == 0 {
			// raw secret
			fmt.Printf("inject secret %q info %s\n", name, secrets)
			err := ioutil.WriteFile(secrets, []byte(value), 0444)
			if err != nil {
				fmt.Fprintf(os.Stderr, err.Error())
				os.Exit(3)
			}
			os.Exit(0)
		}

		var unmarshalled interface{}
		err := json.Unmarshal([]byte(value), &unmarshalled)
		if err == nil {
			if dict, ok := unmarshalled.(map[string]interface{}); ok {
				os.MkdirAll(secrets, 0555)
				for k, v := range dict {
					if !contains(keys, k) && !contains(keys, "*") {
						continue
					}
					path := filepath.Join(secrets, k)
					fmt.Printf("inject secret %q info %s\n", k, path)

					var raw []byte
					if s, ok := v.(string); ok {
						raw = []byte(s)
					} else {
						raw, err = json.Marshal(v)
						if err != nil {
							fmt.Fprintf(os.Stderr, err.Error())
							os.Exit(2)
						}
					}

					err = ioutil.WriteFile(path, raw, 0444)
					if err != nil {
						fmt.Fprintf(os.Stderr, err.Error())
						os.Exit(3)
					}
				}
				os.Exit(0)
			}
		}
	}
}

func contains(keys []string, s string) bool {
	for _, k := range keys {
		if k == s {
			return true
		}
	}
	return false
}
