package main

import (
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (cs *ContainerdSuite) TestStartBusyboxLsSlash(t *check.C) {
	expectedOutput := `bin
dev
etc
home
lib
lib64
linuxrc
media
mnt
opt
proc
root
run
sbin
sys
tmp
usr
var
`
	if err := CreateBusyboxBundle("busybox-ls-slash", []string{"ls", "/"}); err != nil {
		t.Fatal(err)
	}

	c, err := cs.RunContainer("myls", "busybox-ls-slash")
	if err != nil {
		t.Fatal(err)
	}

	t.Assert(c.io.stdoutBuffer.String(), checker.Equals, expectedOutput)
}

func (cs *ContainerdSuite) TestStartBusyboxNoSuchFile(t *check.C) {
	expectedOutput := `oci runtime error: exec: \"NoSuchFile\": executable file not found in $PATH`

	if err := CreateBusyboxBundle("busybox-NoSuchFile", []string{"NoSuchFile"}); err != nil {
		t.Fatal(err)
	}

	_, err := cs.RunContainer("NoSuchFile", "busybox-NoSuchFile")
	t.Assert(err.Error(), checker.Contains, expectedOutput)
}

func (cs *ContainerdSuite) TestStartBusyboxTop(t *check.C) {
	if err := CreateBusyboxBundle("busybox-top", []string{"top"}); err != nil {
		t.Fatal(err)
	}

	_, err := cs.StartContainer("top", "busybox-top")
	t.Assert(err, checker.Equals, nil)
}
