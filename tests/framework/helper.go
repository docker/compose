package framework

import (
	"log"
	"strings"

	"github.com/robpike/filter"
)

func nonEmptyString(s string) bool {
	return strings.TrimSpace(s) != ""
}

//Lines get lines from a raw string
func Lines(output string) []string {
	return filter.Choose(strings.Split(output, "\n"), nonEmptyString).([]string)
}

//Columns get columns from a line
func Columns(line string) []string {
	return filter.Choose(strings.Split(line, " "), nonEmptyString).([]string)
}

// It runs func
func It(description string, test func()) {
	test()
	log.Print("Passed: ", description)
}
