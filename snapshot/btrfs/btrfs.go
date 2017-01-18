package btrfs

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/containerd"
	"github.com/stevvooe/go-btrfs"
)

type Btrfs struct {
	device string // maybe we can resolve it with path?
	root   string // root provides paths for internal storage.
}

func NewBtrfs(device, root string) (*Btrfs, error) {
	return &Btrfs{device: device, root: root}, nil
}

func (lm *Btrfs) Prepare(key, parent string) ([]containerd.Mount, error) {
	active := filepath.Join(lm.root, "active")
	if err := os.MkdirAll(active, 0755); err != nil {
		return nil, err
	}

	dir := filepath.Join(active, hash(key))

	if parent == "" {
		// create new subvolume
		// btrfs subvolume create /dir
		if err := btrfs.SubvolCreate(dir); err != nil {
			return nil, err
		}
	} else {
		// btrfs subvolume snapshot /parent /subvol
		if err := btrfs.SubvolSnapshot(dir, parent, false); err != nil {
			return nil, err
		}
	}

	// get the subvolume id back out for the mount
	info, err := btrfs.SubvolInfo(dir)
	if err != nil {
		return nil, err
	}

	return []containerd.Mount{
		{
			Type:   "btrfs",
			Source: lm.device, // device?
			// NOTE(stevvooe): While it would be nice to use to uuids for
			// mounts, they don't work reliably if the uuids are missing.
			Options: []string{fmt.Sprintf("subvolid=%d", info.ID)},
		},
	}, nil
}

func (lm *Btrfs) Commit(name, key string) error {
	dir := filepath.Join(lm.root, "active", hash(key))

	fmt.Println("commit to", name)
	if err := btrfs.SubvolSnapshot(name, dir, true); err != nil {
		return err
	}

	return btrfs.SubvolDelete(dir)
}

func hash(k string) string {
	return fmt.Sprintf("%x", sha256.Sum224([]byte(k)))
}
