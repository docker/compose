package main

import (
	"log"
	"strings"
	"time"

	"github.com/robpike/filter"

	g "github.com/onsi/gomega"

	f "github.com/docker/api/tests/framework"
)

func main() {
	setup()

	It("ensures context command includes azure-login and aci-create", func() {
		output := f.NewDockerCommand("context", "create", "--help").ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("docker context create CONTEXT BACKEND [OPTIONS] [flags]"))
		g.Expect(output).To(g.ContainSubstring("--aci-location"))
		g.Expect(output).To(g.ContainSubstring("--aci-subscription-id"))
		g.Expect(output).To(g.ContainSubstring("--aci-resource-group"))
	})

	It("should be initialized with default context", func() {
		f.NewCommand("docker", "context", "use", "default").ExecOrDie()
		output := f.NewCommand("docker", "context", "ls").ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("default *"))
	})

	It("should list all legacy commands", func() {
		output := f.NewDockerCommand("--help").ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("swarm"))
	})

	It("should execute legacy commands", func() {
		output, _ := f.NewDockerCommand("swarm", "join").Exec()
		g.Expect(output).To(g.ContainSubstring("\"docker swarm join\" requires exactly 1 argument."))
	})

	It("should run local container in less than 2 secs", func() {
		f.NewDockerCommand("pull", "hello-world").ExecOrDie()
		output := f.NewDockerCommand("run", "hello-world").WithTimeout(time.NewTimer(2 * time.Second).C).ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("Hello from Docker!"))
	})

	It("should list local container", func() {
		output := f.NewDockerCommand("ps", "-a").ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("hello-world"))
	})

	It("creates a new test context to hardcoded example backend", func() {
		f.NewDockerCommand("context", "create", "test-example", "example").ExecOrDie()
		//g.Expect(output).To(g.ContainSubstring("test-example context acitest created"))
	})
	defer f.NewCommand("docker", "context", "rm", "test-example", "-f").ExecOrDie()

	It("uses the test context", func() {
		currentContext := f.NewCommand("docker", "context", "use", "test-example").ExecOrDie()
		g.Expect(currentContext).To(g.ContainSubstring("test-example"))
		output := f.NewCommand("docker", "context", "ls").ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("test-example *"))
	})

	It("can run ps command", func() {
		output := f.NewDockerCommand("ps").ExecOrDie()
		lines := lines(output)
		g.Expect(len(lines)).To(g.Equal(3))
		g.Expect(lines[2]).To(g.ContainSubstring("1234  alpine"))
	})

	It("can run quiet ps command", func() {
		output := f.NewDockerCommand("ps", "-q").ExecOrDie()
		lines := lines(output)
		g.Expect(len(lines)).To(g.Equal(2))
		g.Expect(lines[0]).To(g.Equal("id"))
		g.Expect(lines[1]).To(g.Equal("1234"))
	})

	It("can run 'run' command", func() {
		output := f.NewDockerCommand("run", "nginx", "-p", "80:80").ExecOrDie()
		g.Expect(output).To(g.ContainSubstring("Running container \"nginx\" with name"))
	})
}

func nonEmptyString(s string) bool {
	return strings.TrimSpace(s) != ""
}

func lines(output string) []string {
	return filter.Choose(strings.Split(output, "\n"), nonEmptyString).([]string)
}

// It runs func
func It(description string, test func()) {
	test()
	log.Print("Passed: ", description)
}

func gomegaFailHandler(message string, callerSkip ...int) {
	log.Fatal(message)
}

func setup() {
	g.RegisterFailHandler(gomegaFailHandler)
}
