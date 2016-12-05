package execution

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

const processesDir = "processes"

type StateDir string

func NewStateDir(root, id string) (StateDir, error) {
	path := filepath.Join(root, id)
	err := os.Mkdir(path, 0700)
	if err != nil {
		return "", err
	}

	err = os.Mkdir(filepath.Join(path, processesDir), 0700)
	if err != nil {
		os.RemoveAll(path)
		return "", err
	}

	return StateDir(path), err
}

func (s StateDir) Delete() error {
	return os.RemoveAll(string(s))
}

func (s StateDir) NewProcess(id string) (string, error) {
	// TODO: generate id
	newPath := filepath.Join(string(s), "1")
	err := os.Mkdir(newPath, 0755)
	if err != nil {
		return "", err
	}

	return newPath, nil
}

func (s StateDir) ProcessDir(id string) string {
	return filepath.Join(string(s), id)
}

func (s StateDir) DeleteProcess(id string) error {
	return os.RemoveAll(filepath.Join(string(s), id))
}

func (s StateDir) Processes() ([]string, error) {
	basepath := filepath.Join(string(s), processesDir)
	dirs, err := ioutil.ReadDir(basepath)
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0)
	for _, d := range dirs {

		if d.IsDir() {
			paths = append(paths, filepath.Join(basepath, d.Name()))
		}
	}

	return paths, nil
}
