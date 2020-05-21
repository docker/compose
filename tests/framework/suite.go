/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package framework

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		fmt.Printf("Contents of %s:\n%s\n\nContents of config dir:\n", cp, string(d))
		for _, p := range dirContents(s.ConfigDir) {
			fmt.Println(p)
		}
		s.T().Fail()
	})
	s.linkClassicDocker()
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

func (s *Suite) linkClassicDocker() {
	p, err := exec.LookPath("docker")
	gomega.Expect(err).To(gomega.BeNil())
	err = os.Symlink(p, filepath.Join(s.BinDir, "docker-classic"))
	gomega.Expect(err).To(gomega.BeNil())
	err = os.Setenv("PATH", fmt.Sprintf("%s:%s", s.BinDir, os.Getenv("PATH")))
	gomega.Expect(err).To(gomega.BeNil())
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

// NewCommand creates a command context.
func (s *Suite) NewCommand(command string, args ...string) *CmdContext {
	return &CmdContext{
		command: command,
		args:    args,
		retries: RetriesContext{interval: time.Second},
	}
}

func dockerExecutable() string {
	if runtime.GOOS == "windows" {
		return "../../bin/docker.exe"
	}
	return "../../bin/docker"
}

// NewDockerCommand creates a docker builder.
func (s *Suite) NewDockerCommand(args ...string) *CmdContext {
	return s.NewCommand(dockerExecutable(), args...)
}
