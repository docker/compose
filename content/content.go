package content

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/distribution/digest"
	"github.com/nightlyone/lockfile"
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

type Status struct {
	Ref  string
	Size int64
	Meta interface{}
}

func (cs *ContentStore) Stat(ref string) (Status, error) {
	dfi, err := os.Stat(filepath.Join(cs.root, "ingest", ref, "data"))
	if err != nil {
		return Status{}, err
	}

	return Status{
		Ref:  ref,
		Size: dfi.Size(),
	}, nil
}

func (cs *ContentStore) Active() ([]Status, error) {
	ip := filepath.Join(cs.root, "ingest")

	fp, err := os.Open(ip)
	if err != nil {
		return nil, err
	}

	fis, err := fp.Readdir(-1)
	if err != nil {
		return nil, err
	}

	var active []Status
	for _, fi := range fis {
		stat, err := cs.Stat(fi.Name())
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}

			// TODO(stevvooe): This is a common error if uploads are being
			// completed while making this listing. Need to consider taking a
			// lock on the whole store to coordinate this aspect.
			//
			// Another option is to cleanup downloads asynchronously and
			// coordinate this method with the cleanup process.
			//
			// For now, we just skip them, as they really don't exist.
			continue
		}

		active = append(active, stat)
	}

	return active, nil
}

// TODO(stevvooe): Allow querying the set of blobs in the blob store.

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
	path, data, lock, err := cs.ingestPaths(ref)
	if err != nil {
		return nil, err
	}

	// use single path mkdir for this to ensure ref is only base path, in
	// addition to validation above.
	if err := os.Mkdir(path, 0755); err != nil {
		return nil, err
	}

	if err := tryLock(lock); err != nil {
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
		lock:     lock,
		path:     path,
		digester: digest.Canonical.New(),
	}, nil
}

func (cs *ContentStore) Resume(ref string) (*ContentWriter, error) {
	path, data, lock, err := cs.ingestPaths(ref)
	if err != nil {
		return nil, err
	}

	if err := tryLock(lock); err != nil {
		return nil, err
	}

	digester := digest.Canonical.New()

	// slow slow slow!!, send to goroutine or use resumable hashes
	fp, err := os.Open(data)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	offset, err := io.Copy(digester.Hash(), fp)
	if err != nil {
		return nil, err
	}

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
		lock:     lock,
		path:     path,
		offset:   offset,
		digester: digester,
	}, nil
}

func (cs *ContentStore) ingestPaths(ref string) (string, string, lockfile.Lockfile, error) {
	cref := filepath.Clean(ref)
	if cref != ref {
		return "", "", "", errors.Errorf("invalid path after clean")
	}

	fp := filepath.Join(cs.root, "ingest", ref)

	// ensure we don't escape root
	if !strings.HasPrefix(fp, cs.root) {
		return "", "", "", errors.Errorf("path %q escapes root", ref)
	}

	// ensure we are just a single path component
	if ref != filepath.Base(fp) {
		return "", "", "", errors.Errorf("ref must be a single path component")
	}

	lock, err := lockfile.New(filepath.Join(fp, "lock"))
	if err != nil {
		return "", "", "", errors.Wrap(err, "error creating lockfile")
	}

	return fp, filepath.Join(fp, "data"), lock, nil
}
