package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/docker/ecs-plugin/secrets"
)

const secretsFolder = "/run/secrets"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: secrets <json encoded []Secret>")
		os.Exit(1)
	}

	var input []secrets.Secret
	err := json.Unmarshal([]byte(os.Args[1]), &input)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}

	for _, secret := range input {
		err := secrets.CreateSecretFiles(secret, secretsFolder)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
}
