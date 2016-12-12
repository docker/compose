package snapshot

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/containerd"
)

func NewOverlayfs(root string) (*Overlayfs, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	for _, p := range []string{
		"snapshots",
		"active",
	} {
		if err := os.MkdirAll(filepath.Join(root, p), 0700); err != nil {
			return nil, err
		}
	}
	return &Overlayfs{
		root:  root,
		cache: newCache(),
	}, nil
}

type Overlayfs struct {
	root  string
	cache *cache
}

func (o *Overlayfs) Prepare(key string, parentName string) ([]containerd.Mount, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	active, err := o.newActiveDir(key)
	if err != nil {
		return nil, err
	}
	if parentName != "" {
		if err := active.setParent(parentName); err != nil {
			return nil, err
		}
	}
	return active.mounts(o.cache)
}

func (o *Overlayfs) Commit(key string, name string) error {
	active := o.getActive(key)
	return active.commit(name)
}

func (o *Overlayfs) newActiveDir(key string) (*activeDir, error) {
	var (
		hash = hash(key)
		path = filepath.Join(o.root, "active", hash)
	)
	a := &activeDir{
		path:         path,
		snapshotsDir: filepath.Join(o.root, "snapshots"),
	}
	for _, p := range []string{
		"work",
		"fs",
	} {
		if err := os.MkdirAll(filepath.Join(path, p), 0700); err != nil {
			a.delete()
			return nil, err
		}
	}
	return a, nil
}

func (o *Overlayfs) getActive(key string) *activeDir {
	return &activeDir{
		path:         filepath.Join(o.root, "active", hash(key)),
		snapshotsDir: filepath.Join(o.root, "snapshots"),
	}
}

func validKey(key string) error {
	_, err := filepath.Abs(key)
	return err
}

func hash(k string) string {
	h := md5.New()
	h.Write([]byte(k))
	return hex.EncodeToString(h.Sum(nil))
}

type activeDir struct {
	snapshotsDir string
	path         string
}

func (a *activeDir) delete() error {
	return os.RemoveAll(a.path)
}

func (a *activeDir) setParent(name string) error {
	return os.Symlink(filepath.Join(a.snapshotsDir, name), filepath.Join(a.path, "parent"))
}

func (a *activeDir) commit(name string) error {
	if err := os.RemoveAll(filepath.Join(a.path, "work")); err != nil {
		return err
	}
	return os.Rename(a.path, filepath.Join(a.snapshotsDir, name))
}

func (a *activeDir) mounts(c *cache) ([]containerd.Mount, error) {
	var (
		parents []string
		err     error
		current = a.path
	)
	for {
		if current, err = c.get(current); err != nil {
			if os.IsNotExist(err) {
				break
			}
			return nil, err
		}
		parents = append(parents, filepath.Join(current, "fs"))
	}
	if len(parents) == 0 {
		// if we only have one layer/no parents then just return a bind mount as overlay
		// will not work
		return []containerd.Mount{
			{
				Source: filepath.Join(a.path, "fs"),
				Type:   "bind",
				Options: []string{
					"rw",
					"rbind",
				},
			},
		}, nil
	}
	options := []string{
		fmt.Sprintf("workdir=%s", filepath.Join(a.path, "work")),
		fmt.Sprintf("upperdir=%s", filepath.Join(a.path, "fs")),
		fmt.Sprintf("lowerdir=%s", strings.Join(parents, ":")),
	}
	return []containerd.Mount{
		{
			Type:    "overlay",
			Source:  "overlay",
			Options: options,
		},
	}, nil
}

func newCache() *cache {
	return &cache{
		parents: make(map[string]string),
	}
}

type cache struct {
	mu      sync.Mutex
	parents map[string]string
}

func (c *cache) get(path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	parentRoot, ok := c.parents[path]
	if !ok {
		link, err := os.Readlink(filepath.Join(path, "parent"))
		if err != nil {
			return "", err
		}
		c.parents[path], parentRoot = link, link
	}
	return parentRoot, nil
}
