/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package utils

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
)

func CreateTar(content string, config types.FileReferenceConfig, modTime time.Time) (bytes.Buffer, error) {
	value := []byte(content)
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	mode := types.FileMode(0o444)
	if config.Mode != nil {
		mode = *config.Mode
	}

	var uid, gid int
	if config.UID != "" {
		v, err := strconv.Atoi(config.UID)
		if err != nil {
			return b, err
		}
		uid = v
	}
	if config.GID != "" {
		v, err := strconv.Atoi(config.GID)
		if err != nil {
			return b, err
		}
		gid = v
	}

	header := &tar.Header{
		Name:    config.Target,
		Size:    int64(len(value)),
		Mode:    int64(mode),
		ModTime: modTime,
		Uid:     uid,
		Gid:     gid,
	}
	err := tarWriter.WriteHeader(header)
	if err != nil {
		return bytes.Buffer{}, err
	}
	_, err = tarWriter.Write(value)
	if err != nil {
		return bytes.Buffer{}, err
	}
	err = tarWriter.Close()
	return b, err
}

func CreateTarByPath(path string, modTime time.Time) (*bytes.Buffer, error) {
	b := new(bytes.Buffer)
	tw := tar.NewWriter(b)
	defer func() {
		_ = tw.Close()
	}()

	// Walk the directory or file tree at the given path
	err := filepath.Walk(path, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// Preserve folder structure by using relative paths
		header.Name, err = filepath.Rel(filepath.Dir(path), file)
		if err != nil {
			return err
		}

		// Set custom modification time
		header.ModTime = modTime.UTC()

		// Write header to the tarball
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's a directory, we don't need to write file content
		if fi.Mode().IsRegular() {
			// Open the file and write its contents
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}

		return nil
	})

	return b, err
}
