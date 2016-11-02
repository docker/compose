package content

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/distribution/digest"
	"github.com/pkg/errors"
)

var (
	ErrBlobNotFound = errors.New("blob not found")

	bufPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 32<<10)
		},
	}
)

// ContentStore is digest-keyed store for content. All data written into the
// store is stored under a verifiable digest.
//
// ContentStore can generally support multi-reader, single-writer ingest of
// data, including resumable ingest.
type ContentStore struct {
	root string
}

func OpenContentStore(root string) (*ContentStore, error) {
	if err := os.MkdirAll(filepath.Join(root, "ingest"), 0777); err != nil && !os.IsExist(err) {
		return nil, err
	}

	return &ContentStore{
		root: root,
	}, nil
}

// OpenBlob opens the blob for reading identified by dgst.
//
// The opened blob may also implement seek. Callers can detect with io.Seeker.
func OpenBlob(cs *ContentStore, dgst digest.Digest) (io.ReadCloser, error) {
	path, err := cs.GetPath(dgst)
	if err != nil {
		return nil, err
	}

	fp, err := os.Open(path)
	return fp, err
}

// WriteBlob writes data with the expected digest into the content store. If
// expected already exists, the method returns immediately and the reader will
// not be consumed.
//
// This is useful when the digest and size are known beforehand.
//
// Copy is buffered, so no need to wrap reader in buffered io.
func WriteBlob(cs *ContentStore, r io.Reader, size int64, expected digest.Digest) error {
	cw, err := cs.Begin(expected.Hex())
	if err != nil {
		return err
	}
	buf := bufPool.Get().([]byte)
	defer bufPool.Put(buf)

	nn, err := io.CopyBuffer(cw, r, buf)
	if err != nil {
		return err
	}

	if nn != size {
		return errors.Errorf("failed size verification: %v != %v", nn, size)
	}

	if err := cw.Commit(size, expected); err != nil {
		return err
	}

	return nil
}

func (cs *ContentStore) GetPath(dgst digest.Digest) (string, error) {
	p := filepath.Join(cs.root, "blobs", dgst.Algorithm().String(), dgst.Hex())
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return "", ErrBlobNotFound
		}

		return "", err
	}

	return p, nil
}

// Begin starts a new write transaction against the blob store.
//
// The argument `ref` is used to identify the transaction. It must be a valid
// path component, meaning it has no `/` characters and no `:` (we'll ban
// others fs characters, as needed).
//
// TODO(stevvooe): Figure out minimum common set of characters, basically common
func (cs *ContentStore) Begin(ref string) (*ContentWriter, error) {
	path, data, err := cs.ingestPaths(ref)
	if err != nil {
		return nil, err
	}

	// use single path mkdir for this to ensure ref is only base path, in
	// addition to validation above.
	if err := os.Mkdir(path, 0755); err != nil {
		return nil, err
	}

	fp, err := os.OpenFile(data, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open data file")
	}
	defer fp.Close()

	// re-open the file in append mode
	fp, err = os.OpenFile(data, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, errors.Wrap(err, "error opening for append")
	}

	return &ContentWriter{
		cs:       cs,
		fp:       fp,
		path:     path,
		digester: digest.Canonical.New(),
	}, nil
}

func (cs *ContentStore) Resume(ref string) (*ContentWriter, error) {
	path, data, err := cs.ingestPaths(ref)
	if err != nil {
		return nil, err
	}

	digester := digest.Canonical.New()

	// slow slow slow!!, send to goroutine
	fp, err := os.Open(data)
	offset, err := io.Copy(digester.Hash(), fp)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	fp1, err := os.OpenFile(data, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrap(err, "ingest does not exist")
		}

		return nil, errors.Wrap(err, "error opening for append")
	}

	return &ContentWriter{
		cs:       cs,
		fp:       fp1,
		path:     path,
		offset:   offset,
		digester: digester,
	}, nil
}

func (cs *ContentStore) ingestPaths(ref string) (string, string, error) {
	cref := filepath.Clean(ref)
	if cref != ref {
		return "", "", errors.Errorf("invalid path after clean")
	}

	fp := filepath.Join(cs.root, "ingest", ref)

	// ensure we don't escape root
	if !strings.HasPrefix(fp, cs.root) {
		return "", "", errors.Errorf("path %q escapes root", ref)
	}

	// ensure we are just a single path component
	if ref != filepath.Base(fp) {
		return "", "", errors.Errorf("ref must be a single path component")
	}

	return fp, filepath.Join(fp, "data"), nil
}

// ContentWriter represents a write transaction against the blob store.
//
//
type ContentWriter struct {
	cs       *ContentStore
	fp       *os.File // opened data file
	path     string   // path to writer dir
	offset   int64
	digester digest.Digester
}

func (cw *ContentWriter) Write(p []byte) (n int, err error) {
	n, err = cw.fp.Write(p)
	cw.digester.Hash().Write(p[:n])
	return n, err
}

func (cw *ContentWriter) Commit(size int64, expected digest.Digest) error {
	if err := cw.fp.Sync(); err != nil {
		return errors.Wrap(err, "sync failed")
	}

	fi, err := cw.fp.Stat()
	if err != nil {
		return errors.Wrap(err, "stat on data file failed")
	}

	if size != fi.Size() {
		return errors.Errorf("failed size validation: %v != %v", fi.Size(), size)
	}

	dgst := cw.digester.Digest()
	if expected != dgst {
		return errors.Errorf("unexpected digest: %v != %v", dgst, expected)
	}

	apath := filepath.Join(cw.cs.root, "blobs", dgst.Algorithm().String())
	if err := os.MkdirAll(apath, 0755); err != nil {
		return err
	}

	dpath := filepath.Join(apath, dgst.Hex())

	// clean up!!
	defer os.RemoveAll(cw.path)
	return os.Rename(filepath.Join(cw.path, "data"), dpath)
}

// Close the writer, leaving the progress in tact.
//
// If one needs to resume the transaction, a new writer can be obtained from
// `ContentStore.Resume` using the same key.
func (cw *ContentWriter) Close() error {
	return cw.fp.Close()
}
