package content

import (
	"bufio"
	"bytes"
	"crypto/rand"
	_ "crypto/sha256" // required for digest package
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/docker/distribution/digest"
)

func TestContentWriter(t *testing.T) {
	tmpdir, cs, cleanup := contentStoreEnv(t)
	defer cleanup()

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

	// make sure that second resume also fails
	if _, err = cs.Resume("myref"); err == nil {
		// TODO(stevvooe): This also works across processes. Need to find a way
		// to test that, as well.
		t.Fatal("no error on second resume")
	}

	// we should also see this as an active ingestion
	ingestions, err := cs.Active()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(ingestions, []Status{
		{
			Ref:  "myref",
			Size: 0,
		},
	}) {
		t.Fatalf("unexpected ingestion set: %v", ingestions)
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

	path := checkBlobPath(t, cs, expected)

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

func TestWalkBlobs(t *testing.T) {
	_, cs, cleanup := contentStoreEnv(t)
	defer cleanup()

	const (
		nblobs  = 4 << 10
		maxsize = 4 << 10
	)

	var (
		blobs    = map[digest.Digest][]byte{}
		expected = map[digest.Digest]struct{}{}
		found    = map[digest.Digest]struct{}{}
	)

	for i := 0; i < nblobs; i++ {
		p := make([]byte, mrand.Intn(maxsize))

		if _, err := rand.Read(p); err != nil {
			t.Fatal(err)
		}

		dgst := checkWrite(t, cs, p)
		blobs[dgst] = p
		expected[dgst] = struct{}{}
	}

	if err := cs.Walk(func(path string, dgst digest.Digest) error {
		found[dgst] = struct{}{}
		if checked := checkBlobPath(t, cs, dgst); checked != path {
			t.Fatalf("blob path did not match: %v != %v", path, checked)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expected, found) {
		t.Fatalf("expected did not match found: %v != %v", found, expected)
	}
}

func contentStoreEnv(t interface {
	Fatal(args ...interface{})
}) (string, *ContentStore, func()) {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	fn := runtime.FuncForPC(pc)

	tmpdir, err := ioutil.TempDir("", filepath.Base(fn.Name())+"-")
	if err != nil {
		t.Fatal(err)
	}

	cs, err := OpenContentStore(tmpdir)
	if err != nil {
		os.RemoveAll(tmpdir)
		t.Fatal(err)
	}

	return tmpdir, cs, func() {
		os.RemoveAll(tmpdir)
	}
}

func checkCopy(t interface {
	Fatal(args ...interface{})
}, size int64, dst io.Writer, src io.Reader) {
	nn, err := io.Copy(dst, src)
	if err != nil {
		t.Fatal(err)
	}

	if nn != size {
		t.Fatal("incorrect number of bytes copied")
	}
}

func checkBlobPath(t *testing.T, cs *ContentStore, dgst digest.Digest) string {
	path, err := cs.GetPath(dgst)
	if err != nil {
		t.Fatal(err, dgst)
	}
	if path != filepath.Join(cs.root, "blobs", dgst.Algorithm().String(), dgst.Hex()) {
		t.Fatalf("unxpected path: %q", path)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("error stating blob path: %v", err)
	}

	// ensure that only read bits are set.
	if ((fi.Mode() & os.ModePerm) & 0333) != 0 {
		t.Fatalf("incorrect permissions: %v", fi.Mode())
	}

	return path
}

func checkWrite(t *testing.T, cs *ContentStore, p []byte) digest.Digest {
	dgst := digest.FromBytes(p)
	if err := WriteBlob(cs, bytes.NewReader(p), int64(len(p)), dgst); err != nil {
		t.Fatal(err)
	}

	return dgst
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
