package framework

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/robpike/filter"

	"github.com/onsi/gomega"
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

func gomegaFailHandler(message string, callerSkip ...int) {
	log.Fatal(message)
}

//SetupTest Init gomega fail handler
func SetupTest() {
	gomega.RegisterFailHandler(gomegaFailHandler)

	linkClassicDocker()
}

func linkClassicDocker() {
	dockerOriginal := strings.TrimSuffix(NewCommand("which", "docker").ExecOrDie(), "\n")
	_, err := NewCommand("rm", "-r", "./bin/tests").Exec()
	if err == nil {
		fmt.Println("Removing existing /bin/tests folder before running tests")
	}
	_, err = NewCommand("mkdir", "-p", "./bin/tests").Exec()
	gomega.Expect(err).To(gomega.BeNil())
	NewCommand("ln", "-s", dockerOriginal, "./bin/tests/docker-classic").ExecOrDie()
	newPath := "./bin/tests:" + os.Getenv("PATH")
	err = os.Setenv("PATH", newPath)
	gomega.Expect(err).To(gomega.BeNil())
}
