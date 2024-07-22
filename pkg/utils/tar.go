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
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
)

func CreateTar(content []byte, config types.FileReferenceConfig, modTime time.Time) (bytes.Buffer, error) {
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	mode := uint32(0o444)
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
		Size:    int64(len(content)),
		Mode:    int64(mode),
		ModTime: modTime,
		Uid:     uid,
		Gid:     gid,
	}
	err := tarWriter.WriteHeader(header)
	if err != nil {
		return bytes.Buffer{}, err
	}
	_, err = tarWriter.Write(content)
	if err != nil {
		return bytes.Buffer{}, err
	}
	err = tarWriter.Close()
	return b, err
}
