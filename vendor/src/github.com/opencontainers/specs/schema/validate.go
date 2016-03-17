package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xeipuuv/gojsonschema"
)

func main() {
	if len(os.Args[1:]) != 2 {
		fmt.Printf("ERROR: usage is: %s <schema.json> <config.json>", os.Args[0])
		os.Exit(1)
	}

	schemaPath, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	documentPath, err := filepath.Abs(os.Args[2])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	schemaLoader := gojsonschema.NewReferenceLoader("file://" + schemaPath)
	documentLoader := gojsonschema.NewReferenceLoader("file://" + documentPath)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		panic(err.Error())
	}

	if result.Valid() {
		fmt.Printf("The document is valid\n")
	} else {
		fmt.Printf("The document is not valid. see errors :\n")
		for _, desc := range result.Errors() {
			fmt.Printf("- %s\n", desc)
		}
		os.Exit(1)
	}
}
