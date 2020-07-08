/*
   Copyright 2020 Docker, Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package framework

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

// Suite is used to store context information for e2e tests
type Suite struct {
	suite.Suite
	ConfigDir string
	BinDir    string
}

// SetupSuite is run before running any tests
func (s *Suite) SetupSuite() {
	d, _ := ioutil.TempDir("", "")
	s.BinDir = d
	gomega.RegisterFailHandler(func(message string, callerSkip ...int) {
		log.Error(message)
		cp := filepath.Join(s.ConfigDir, "config.json")
		d, _ := ioutil.ReadFile(cp)
		fmt.Printf("Bin dir:%s\n", s.BinDir)
		fmt.Printf("Contents of %s:\n%s\n\nContents of config dir:\n", cp, string(d))
		for _, p := range dirContents(s.ConfigDir) {
			fmt.Println(p)
		}
		s.T().Fail()
	})
	s.copyExecutablesInBinDir()
}

// TearDownSuite is run after all tests
func (s *Suite) TearDownSuite() {
	_ = os.RemoveAll(s.BinDir)
}

func dirContents(dir string) []string {
	res := []string{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		res = append(res, filepath.Join(dir, path))
		return nil
	})
	return res
}

func (s *Suite) copyExecutablesInBinDir() {
	p, err := exec.LookPath(DockerClassicExecutable())
	if err != nil {
		p, err = exec.LookPath(dockerExecutable())
	}
	gomega.Expect(err).To(gomega.BeNil())
	err = copyFile(p, filepath.Join(s.BinDir, DockerClassicExecutable()))
	gomega.Expect(err).To(gomega.BeNil())
	dockerPath, err := filepath.Abs("../../bin/" + dockerExecutable())
	gomega.Expect(err).To(gomega.BeNil())
	err = copyFile(dockerPath, filepath.Join(s.BinDir, dockerExecutable()))
	gomega.Expect(err).To(gomega.BeNil())
	err = os.Setenv("PATH", concatenatePath(s.BinDir))
	gomega.Expect(err).To(gomega.BeNil())
}

func concatenatePath(path string) string {
	if IsWindows() {
		return fmt.Sprintf("%s;%s", path, os.Getenv("PATH"))
	}
	return fmt.Sprintf("%s:%s", path, os.Getenv("PATH"))
}

func copyFile(sourceFile string, destinationFile string) error {
	input, err := ioutil.ReadFile(sourceFile)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(destinationFile, input, 0777)
	if err != nil {
		return err
	}
	return nil
}

// BeforeTest is run before each test
func (s *Suite) BeforeTest(suite, test string) {
	d, _ := ioutil.TempDir("", "")
	s.ConfigDir = d
	_ = os.Setenv("DOCKER_CONFIG", s.ConfigDir)
}

// AfterTest is run after each test
func (s *Suite) AfterTest(suite, test string) {
	_ = os.RemoveAll(s.ConfigDir)
}

// ListProcessesCommand creates a command to list processes, "tasklist" on windows, "ps" otherwise.
func (s *Suite) ListProcessesCommand() *CmdContext {
	if IsWindows() {
		return s.NewCommand("tasklist")
	}
	return s.NewCommand("ps", "-x")
}

// NewCommand creates a command context.
func (s *Suite) NewCommand(command string, args ...string) *CmdContext {
	return &CmdContext{
		command: command,
		args:    args,
		retries: RetriesContext{interval: time.Second},
	}
}

// Step runs a step in a test, with an identified name and output in test results
func (s *Suite) Step(name string, test func()) {
	s.T().Run(name, func(t *testing.T) {
		test()
	})
}

func dockerExecutable() string {
	if IsWindows() {
		return "docker.exe"
	}
	return "docker"
}

// DockerClassicExecutable binary name based on platform
func DockerClassicExecutable() string {
	const comDockerCli = "com.docker.cli"
	if IsWindows() {
		return comDockerCli + ".exe"
	}
	return comDockerCli
}

// NewDockerCommand creates a docker builder.
func (s *Suite) NewDockerCommand(args ...string) *CmdContext {
	return s.NewCommand(dockerExecutable(), args...)
}
