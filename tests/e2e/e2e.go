package main

import (
	"time"

	. "github.com/docker/api/tests/framework"
	. "github.com/onsi/gomega"
)

func main() {
	SetupTest()

	It("ensures context command includes azure-login and aci-create", func() {
		output := NewDockerCommand("context", "create", "--help").ExecOrDie()
		Expect(output).To(ContainSubstring("docker context create CONTEXT BACKEND [OPTIONS] [flags]"))
		Expect(output).To(ContainSubstring("--aci-location"))
		Expect(output).To(ContainSubstring("--aci-subscription-id"))
		Expect(output).To(ContainSubstring("--aci-resource-group"))
	})

	It("should be initialized with default context", func() {
		NewCommand("docker", "context", "use", "default").ExecOrDie()
		output := NewCommand("docker", "context", "ls").ExecOrDie()
		Expect(output).To(Not(ContainSubstring("test-example")))
		Expect(output).To(ContainSubstring("default *"))
	})

	It("should list all legacy commands", func() {
		output := NewDockerCommand("--help").ExecOrDie()
		Expect(output).To(ContainSubstring("swarm"))
	})

	It("should execute legacy commands", func() {
		output, _ := NewDockerCommand("swarm", "join").Exec()
		Expect(output).To(ContainSubstring("\"docker swarm join\" requires exactly 1 argument."))
	})

	It("should run local container in less than 2 secs", func() {
		NewDockerCommand("pull", "hello-world").ExecOrDie()
		output := NewDockerCommand("run", "hello-world").WithTimeout(time.NewTimer(2 * time.Second).C).ExecOrDie()
		Expect(output).To(ContainSubstring("Hello from Docker!"))
	})

	It("should list local container", func() {
		output := NewDockerCommand("ps", "-a").ExecOrDie()
		Expect(output).To(ContainSubstring("hello-world"))
	})

	It("creates a new test context to hardcoded example backend", func() {
		NewDockerCommand("context", "create", "test-example", "example").ExecOrDie()
		//Expect(output).To(ContainSubstring("test-example context acitest created"))
	})
	defer NewCommand("docker", "context", "rm", "test-example", "-f").ExecOrDie()

	It("uses the test context", func() {
		currentContext := NewCommand("docker", "context", "use", "test-example").ExecOrDie()
		Expect(currentContext).To(ContainSubstring("test-example"))
		output := NewCommand("docker", "context", "ls").ExecOrDie()
		Expect(output).To(ContainSubstring("test-example *"))
	})

	It("can run ps command", func() {
		output := NewDockerCommand("ps").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(3))
		Expect(lines[2]).To(ContainSubstring("1234  alpine"))
	})

	It("can run quiet ps command", func() {
		output := NewDockerCommand("ps", "-q").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(2))
		Expect(lines[0]).To(Equal("id"))
		Expect(lines[1]).To(Equal("1234"))
	})

	It("can run 'run' command", func() {
		output := NewDockerCommand("run", "nginx", "-p", "80:80").ExecOrDie()
		Expect(output).To(ContainSubstring("Running container \"nginx\" with name"))
	})
}
