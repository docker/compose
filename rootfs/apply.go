package rootfs

import (
	"io"
	"io/ioutil"

	"github.com/docker/containerd"
	"github.com/docker/containerd/log"
	"github.com/docker/docker/pkg/archive"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

type Snapshotter interface {
	Prepare(key, parent string) ([]containerd.Mount, error)
	Commit(name, key string) error
	Rollback(key string) error
	Exists(name string) bool
}

type Mounter interface {
	Mount(mounts ...containerd.Mount) error
	Unmount(mounts ...containerd.Mount) error
}

// ApplyLayer applies the layer to the provided parent. The resulting snapshot
// will be stored under its ChainID.
//
// The parent *must* be the chainID of the parent layer.
//
// The returned digest is the diffID for the applied layer.
func ApplyLayer(snapshots Snapshotter, mounter Mounter, rd io.Reader, parent digest.Digest) (digest.Digest, error) {
	digester := digest.Canonical.Digester() // used to calculate diffID.
	rd = io.TeeReader(rd, digester)

	// create a temporary directory to work from, needs to be on same
	// filesystem. Probably better if this shared but we'll use a tempdir, for
	// now.
	dir, err := ioutil.TempDir("", "unpack-")
	if err != nil {
		return errors.Wrapf(err, "creating temporary directory failed")
	}

	// TODO(stevvooe): Choose this key WAY more carefully. We should be able to
	// create collisions for concurrent, conflicting unpack processes but we
	// would need to have it be a function of the parent diffID and child
	// layerID (since we don't know the diffID until we are done!).
	key := dir

	mounts, err := snapshots.Prepare(key, parent.String())
	if err != nil {
		return "", err
	}

	if err := mounter.Mount(mounts...); err != nil {
		if err := snapshots.Rollback(key); err != nil {
			log.L.WithError(err).Error("snapshot rollback failed")
		}
		return "", err
	}
	defer mounter.Unmount(mounts...)

	if err := archive.ApplyLayer(key, rd); err != nil {
		return "", err
	}

	diffID := digest.Digest()

	chainID := diffID
	if parent != "" {
		chainID = identity.ChainID([]digest.Digest{parent, chainID})
	}

	return diffID, snapshots.Commit(chainID.String(), key)
}

// Prepare the root filesystem from the set of layers. Snapshots are created
// for each layer if they don't exist, keyed by their chain id. If the snapshot
// already exists, it will be skipped.
//
// If successful, the chainID for the top-level layer is returned. That
// identifier can be used to check out a snapshot.
func Prepare(snapshots Snaphotter, mounter Mounter, layers []ocispec.Descriptor,
	// TODO(stevvooe): The following functions are candidate for internal
	// object functions. We can use these to formulate the beginnings of a
	// rootfs Controller.
	//
	// Just pass them in for now.
	openBlob func(digest.Digest) (digest.Digest, error),
	resolveDiffID func(digest.Digest) digest.Digest,
	registerDiffID func(diffID, dgst digest.Digest) error) (digest.Digest, error) {
	var (
		parent digest.Digest
		chain  []digest.Digest
	)

	for _, layer := range layers {
		// This will convert a possibly compressed layer hash to the
		// uncompressed hash, if we know about it. If we don't, we unpack and
		// calculate it. If we do have it, we then calculate the chain id for
		// the application and see if the snapshot is there.
		diffID := resolveDiffID(layer.Digest)
		if diffID != "" {
			chainLocal := append(chain, diffID)
			chainID := identity.ChainID(chainLocal)

			if snapshots.Exists(chainID.String()) {
				continue
			}
		}

		rc, err := openBlob(layer.Digest)
		if err != nil {
			return "", err
		}
		defer rc.Close() // pretty lazy!

		diffID, err = ApplyLayer(snapshots, mounter, rc, parent)
		if err != nil {
			return "", err
		}

		// Register the association between the diffID and the layer's digest.
		// For uncompressed layers, this will be the same. For compressed
		// layers, we can look up the diffID from the digest if we've already
		// unpacked it.
		if err := registerDiffID(diffID, layer.Digest); err != nil {
			return nil, err
		}

		chain = append(chain, diffID)
		parent = identity.ChainID(chain)
	}

	return parent, nil
}
