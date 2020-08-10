package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

type secret struct {
	name string
	keys []string
}

const secretsFolder = "/run/secrets"

func main() {
	secrets := parseInput(os.Args[1:])

	for _, secret := range secrets {
		err := createSecretFiles(secret, secretsFolder)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
}

func createSecretFiles(secret secret, path string) error {
	value, ok := os.LookupEnv(secret.name)
	if !ok {
		return fmt.Errorf("%q variable not set", secret.name)
	}

	secrets := filepath.Join(path, secret.name)

	if len(secret.keys) == 0 {
		// raw secret
		fmt.Printf("inject secret %q info %s\n", secret.name, secrets)
		return ioutil.WriteFile(secrets, []byte(value), 0444)
	}

	var unmarshalled interface{}
	err := json.Unmarshal([]byte(value), &unmarshalled)
	if err != nil {
		return errors.Wrapf(err, "%q secret is not a valid JSON document", secret.name)
	}

	dict, ok := unmarshalled.(map[string]interface{})
	if !ok {
		return errors.Wrapf(err, "%q secret is not a JSON dictionary", secret.name)
	}
	err = os.MkdirAll(secrets, 0755)
	if err != nil {
		return err
	}

	if contains(secret.keys, "*") {
		var keys []string
		for k := range dict {
			keys = append(keys, k)
		}
		secret.keys = keys
	}

	for _, k := range secret.keys {
		path := filepath.Join(secrets, k)
		fmt.Printf("inject secret %q info %s\n", k, path)

		v, ok := dict[k]
		if !ok {
			return fmt.Errorf("%q secret has no %q key", secret.name, k)
		}

		var raw []byte
		if s, ok := v.(string); ok {
			raw = []byte(s)
		} else {
			raw, err = json.Marshal(v)
			if err != nil {
				return err
			}
		}

		err = ioutil.WriteFile(path, raw, 0444)
		if err != nil {
			return err
		}
	}
	return nil
}

// parseInput parse secret to be dumped into secret files with syntax `VARIABLE_NAME[:COMA_SEPARATED_KEYS]`
func parseInput(input []string) []secret {
	var secrets []secret
	for _, name := range input {
		i := strings.Index(name, ":")
		var keys []string
		if i > 0 {
			keys = strings.Split(name[i+1:], ",")
			name = name[:i]
		}
		secrets = append(secrets, secret{
			name: name,
			keys: keys,
		})
	}
	return secrets
}

func contains(keys []string, s string) bool {
	for _, k := range keys {
		if k == s {
			return true
		}
	}
	return false
}
