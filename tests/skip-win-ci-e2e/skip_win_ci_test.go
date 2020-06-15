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

package main

import (
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	. "github.com/docker/api/tests/framework"
)

type NonWinCIE2eSuite struct {
	Suite
}

func (s *NonWinCIE2eSuite) TestKillChildOnCancel() {
	It("should kill docker-classic if parent command is cancelled", func() {
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
		err := WaitFor(time.Second, 10*time.Second, errs, func() bool {
			out := s.ListProcessesCommand().ExecOrDie()
			return strings.Contains(out, imageName)
		})
		Expect(err).NotTo(HaveOccurred())
		log.Println("Killing docker process")

		close(shutdown)
		err = WaitFor(time.Second, 12*time.Second, nil, func() bool {
			out := s.ListProcessesCommand().ExecOrDie()
			return !strings.Contains(out, imageName)
		})
		Expect(err).NotTo(HaveOccurred())
	})
}

func TestNonWinCIE2(t *testing.T) {
	suite.Run(t, new(NonWinCIE2eSuite))
}
