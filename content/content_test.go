package content

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/distribution/digest"
)

func TestContentWriter(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "TestContentWriter-")

	cs, err := OpenContentStore(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(tmpdir, "ingest")); os.IsNotExist(err) {
		t.Fatal("ingest dir should be created", err)
	}

	cw, err := cs.Begin("myref")
	if err != nil {
		t.Fatal(err)
	}
	if err := cw.Close(); err != nil {
		t.Fatal(err)
	}

	// try to begin again with same ref, should fail
	cw, err = cs.Begin("myref")
	if err == nil {
		t.Fatal("expected error on repeated begin")
	}

	// reopen, so we can test things
	cw, err = cs.Resume("myref")
	if err != nil {
		t.Fatal(err)
	}

	p := make([]byte, 4<<20)
	if _, err := rand.Read(p); err != nil {
		t.Fatal(err)
	}
	expected := digest.FromBytes(p)

	checkCopy(t, int64(len(p)), cw, bufio.NewReader(ioutil.NopCloser(bytes.NewReader(p))))

	if err := cw.Commit(int64(len(p)), expected); err != nil {
		t.Fatal(err)
	}

	if err := cw.Close(); err != nil {
		t.Fatal(err)
	}

	cw, err = cs.Begin("aref")
	if err != nil {
		t.Fatal(err)
	}

	// now, attempt to write the same data again
	checkCopy(t, int64(len(p)), cw, bufio.NewReader(ioutil.NopCloser(bytes.NewReader(p))))
	if err := cw.Commit(int64(len(p)), expected); err != nil {
		t.Fatal(err)
	}

	path, err := cs.GetPath(expected)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(tmpdir, "blobs", expected.Algorithm().String(), expected.Hex()) {
		t.Fatalf("unxpected path: %q", path)
	}

	// read the data back, make sure its the same
	pp, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(p, pp) {
		t.Fatal("mismatched data written to disk")
	}

	dumpDir(tmpdir)
}

func checkCopy(t *testing.T, size int64, dst io.Writer, src io.Reader) {
	nn, err := io.Copy(dst, src)
	if err != nil {
		t.Fatal(err)
	}

	if nn != size {
		t.Fatal("incorrect number of bytes copied")
	}
}

func dumpDir(root string) error {
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		fmt.Println(fi.Mode(), path)
		return nil
	})
}
