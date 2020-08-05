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

package main

import (
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/api/cli/mobycli"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	. "github.com/docker/api/tests/framework"
)

type NonWinCIE2eSuite struct {
	Suite
}

func (s *NonWinCIE2eSuite) TestKillChildOnCancel() {
	s.Step("should kill com.docker.cli if parent command is cancelled", func() {
		imageName := "test-sleep-image"
		out := s.ListProcessesCommand().ExecOrDie()
		Expect(out).NotTo(ContainSubstring(imageName))

		dir := s.ConfigDir
		Expect(ioutil.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(`FROM alpine:3.10
RUN sleep 100`), 0644)).To(Succeed())
		shutdown := make(chan time.Time)
		errs := make(chan error)
		ctx := s.NewDockerCommand("build", "--no-cache", "-t", imageName, ".").WithinDirectory(dir).WithTimeout(shutdown)
		go func() {
			_, err := ctx.Exec()
			errs <- err
		}()
		mobyBuild := mobycli.ComDockerCli + " build --no-cache -t " + imageName
		err := WaitFor(time.Second, 10*time.Second, errs, func() bool {
			out := s.ListProcessesCommand().ExecOrDie()
			return strings.Contains(out, mobyBuild)
		})
		Expect(err).NotTo(HaveOccurred())
		log.Println("Killing docker process")

		close(shutdown)
		err = WaitFor(time.Second, 12*time.Second, nil, func() bool {
			out := s.ListProcessesCommand().ExecOrDie()
			return !strings.Contains(out, mobyBuild)
		})
		Expect(err).NotTo(HaveOccurred())
	})
}

func TestNonWinCIE2(t *testing.T) {
	suite.Run(t, new(NonWinCIE2eSuite))
}
