package execution

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const processesDirName = "processes"

type StateDir string

func LoadStateDir(root, id string) (StateDir, error) {
	path := filepath.Join(root, id)
	if _, err := os.Stat(path); err != nil {
		return "", errors.Wrap(err, "could not find container statedir")
	}
	return StateDir(path), nil
}

func NewStateDir(root, id string) (StateDir, error) {
	path := filepath.Join(root, id)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", errors.Wrap(err, "could not create container statedir")
	}
	if err := os.Mkdir(StateDir(path).processesDir(), 0700); err != nil {
		os.RemoveAll(path)
		return "", errors.Wrap(err, "could not create processes statedir")
	}
	return StateDir(path), nil
}

func (s StateDir) Delete() error {
	err := os.RemoveAll(string(s))
	if err != nil {
		return errors.Wrapf(err, "failed to remove statedir %s", string(s))
	}
	return nil
}

func (s StateDir) NewProcess(id string) (dir string, err error) {
	dir = filepath.Join(s.processesDir(), id)
	if err = os.Mkdir(dir, 0700); err != nil {
		return "", errors.Wrap(err, "could not create process statedir")
	}

	return dir, nil
}

func (s StateDir) ProcessDir(id string) string {
	return filepath.Join(s.processesDir(), id)
}

func (s StateDir) DeleteProcess(id string) error {
	err := os.RemoveAll(filepath.Join(s.processesDir(), id))
	if err != nil {
		return errors.Wrapf(err, "failed to remove process %d statedir", id)
	}
	return nil
}

func (s StateDir) Processes() ([]string, error) {
	procsDir := s.processesDir()
	dirs, err := ioutil.ReadDir(procsDir)
	if err != nil {
		return nil, errors.Wrap(err, "could not list processes statedir")
	}

	paths := make([]string, 0)
	for _, d := range dirs {
		if d.IsDir() {
			paths = append(paths, filepath.Join(procsDir, d.Name()))
		}
	}
	return paths, nil
}

func (s StateDir) processesDir() string {
	return filepath.Join(string(s), processesDirName)
}
