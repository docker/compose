package naive

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/containerd"
	"github.com/docker/docker/pkg/archive"
	"github.com/pkg/errors"
)

type Naive struct {
	root string

	// TODO(stevvooe): Very lazy copying from the overlay driver. We'll have to
	// put *all* of this on disk in an easy to access format.
	active  map[string]activeNaiveSnapshot
	parents map[string]string // mirror of what is on disk
}

type activeNaiveSnapshot struct {
	parent   string
	metadata string
}

func NewNaive(root string) (*Naive, error) {
	if err := os.MkdirAll(root, 0777); err != nil {
		return nil, err
	}

	// TODO(stevvooe): Recover active transactions.

	return &Naive{
		root:    root,
		active:  make(map[string]activeNaiveSnapshot),
		parents: make(map[string]string),
	}, nil
}

// Prepare works per the snapshot specification.
//
// For the naive driver, the data is checked out directly into dst and no
// mounts are returned.
func (lm *Naive) Prepare(dst, parent string) ([]containerd.Mount, error) {
	metadataRoot, err := ioutil.TempDir(lm.root, "active-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to created transaction dir")
	}

	// TODO(stevvooe): Write in driver metadata so it can be identified,
	// probably part of common manager type.

	if err := ioutil.WriteFile(filepath.Join(metadataRoot, "target"), []byte(dst), 0777); err != nil {
		return nil, errors.Wrap(err, "failed to write target to disk")
	}

	if parent != "" {
		if _, ok := lm.parents[parent]; !ok {
			return nil, errors.Wrap(err, "specified parent does not exist")
		}

		if err := ioutil.WriteFile(filepath.Join(metadataRoot, "parent"), []byte(parent), 0777); err != nil {
			return nil, errors.Wrap(err, "error specifying parent")
		}

		// Now, we copy the parent filesystem, just a directory, into dst.
		if err := archive.CopyWithTar(filepath.Join(parent, "data"), dst); err != nil { // note: src, dst args, ick!
			return nil, errors.Wrap(err, "copying of parent failed")
		}
	}

	lm.active[dst] = activeNaiveSnapshot{
		parent:   parent,
		metadata: metadataRoot,
	}

	return nil, nil // no mounts!!
}

// Commit just moves the metadata directory to the diff location.
func (lm *Naive) Commit(diff, dst string) error {
	active, ok := lm.active[dst]
	if !ok {
		return errors.Errorf("%v is not an active transaction", dst)
	}

	// Move the data into our metadata directory, we could probably save disk
	// space if we just saved the diff, but let's get something working.
	if err := archive.CopyWithTar(dst, filepath.Join(active.metadata, "data")); err != nil { // note: src, dst args, ick!
		return errors.Wrap(err, "copying of parent failed")
	}

	if err := os.Rename(active.metadata, diff); err != nil {
		return errors.Wrap(err, "failed to rename metadata into diff")
	}

	lm.parents[diff] = active.parent
	delete(lm.active, dst)

	return nil
}

func (lm *Naive) Rollback(dst string) error {
	active, ok := lm.active[dst]
	if !ok {
		return fmt.Errorf("%q must be an active snapshot", dst)
	}

	delete(lm.active, dst)
	return os.RemoveAll(active.metadata)
}

func (lm *Naive) Parent(diff string) string {
	return lm.parents[diff]
}
