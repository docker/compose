package content

import (
	"errors"
	"sync"

	"github.com/nightlyone/lockfile"
)

// In addition to providing inter-process locks for content ingest, we also
// define a global in process lock to prevent two goroutines writing to the
// same file.
//
// This is prety unsophisticated for now. In the future, we'd probably like to
// have more information about who is holding which locks, as well as better
// error reporting.

var (
	// locks lets us lock in process, as well as output of process.
	locks   = map[lockfile.Lockfile]struct{}{}
	locksMu sync.Mutex
)

func tryLock(lock lockfile.Lockfile) error {
	locksMu.Lock()
	defer locksMu.Unlock()

	if _, ok := locks[lock]; ok {
		return errors.New("file in use")
	}

	if err := lock.TryLock(); err != nil {
		return err
	}

	locks[lock] = struct{}{}
	return nil
}

func unlock(lock lockfile.Lockfile) error {
	locksMu.Lock()
	defer locksMu.Unlock()

	if _, ok := locks[lock]; !ok {
		return nil
	}

	delete(locks, lock)
	return lock.Unlock()
}
