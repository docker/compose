package execution

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

const processesDirName = "processes"

type StateDir string

func NewStateDir(root, id string) (StateDir, error) {
	path := filepath.Join(root, id)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", err
	}
	if err := os.Mkdir(StateDir(path).processesDir(), 0700); err != nil {
		os.RemoveAll(path)
		return "", err
	}
	return StateDir(path), nil
}

func (s StateDir) Delete() error {
	return os.RemoveAll(string(s))
}

func (s StateDir) NewProcess() (id, dir string, err error) {
	dir, err = ioutil.TempDir(s.processesDir(), "")
	if err != nil {
		return "", "", err
	}

	return filepath.Base(dir), dir, err
}

func (s StateDir) ProcessDir(id string) string {
	return filepath.Join(s.processesDir(), id)
}

func (s StateDir) DeleteProcess(id string) error {
	return os.RemoveAll(filepath.Join(s.processesDir(), id))
}

func (s StateDir) Processes() ([]string, error) {
	procsDir := s.processesDir()
	dirs, err := ioutil.ReadDir(procsDir)
	if err != nil {
		return nil, err
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
