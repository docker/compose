/*
   Copyright 2020 The Compose Specification Authors.

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

package loader

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
)

const endOfSpec = rune(0)

// ParseVolume parses a volume spec without any knowledge of the target platform
func ParseVolume(spec string) (types.ServiceVolumeConfig, error) {
	volume := types.ServiceVolumeConfig{}

	switch len(spec) {
	case 0:
		return volume, errors.New("invalid empty volume spec")
	case 1, 2:
		volume.Target = spec
		volume.Type = types.VolumeTypeVolume
		return volume, nil
	}

	var buffer []rune
	for _, char := range spec + string(endOfSpec) {
		switch {
		case isWindowsDrive(buffer, char):
			buffer = append(buffer, char)
		case char == ':' || char == endOfSpec:
			if err := populateFieldFromBuffer(char, buffer, &volume); err != nil {
				populateType(&volume)
				return volume, errors.Wrapf(err, "invalid spec: %s", spec)
			}
			buffer = nil
		default:
			buffer = append(buffer, char)
		}
	}

	populateType(&volume)
	return volume, nil
}

func isWindowsDrive(buffer []rune, char rune) bool {
	return char == ':' && len(buffer) == 1 && unicode.IsLetter(buffer[0])
}

func populateFieldFromBuffer(char rune, buffer []rune, volume *types.ServiceVolumeConfig) error {
	strBuffer := string(buffer)
	switch {
	case len(buffer) == 0:
		return errors.New("empty section between colons")
	// Anonymous volume
	case volume.Source == "" && char == endOfSpec:
		volume.Target = strBuffer
		return nil
	case volume.Source == "":
		volume.Source = strBuffer
		return nil
	case volume.Target == "":
		volume.Target = strBuffer
		return nil
	case char == ':':
		return errors.New("too many colons")
	}
	for _, option := range strings.Split(strBuffer, ",") {
		switch option {
		case "ro":
			volume.ReadOnly = true
		case "rw":
			volume.ReadOnly = false
		case "nocopy":
			volume.Volume = &types.ServiceVolumeVolume{NoCopy: true}
		default:
			if isBindOption(option) {
				setBindOption(volume, option)
			}
			// ignore unknown options
		}
	}
	return nil
}

var Propagations = []string{
	types.PropagationRPrivate,
	types.PropagationPrivate,
	types.PropagationRShared,
	types.PropagationShared,
	types.PropagationRSlave,
	types.PropagationSlave,
}

type setBindOptionFunc func(bind *types.ServiceVolumeBind, option string)

var bindOptions = map[string]setBindOptionFunc{
	types.PropagationRPrivate: setBindPropagation,
	types.PropagationPrivate:  setBindPropagation,
	types.PropagationRShared:  setBindPropagation,
	types.PropagationShared:   setBindPropagation,
	types.PropagationRSlave:   setBindPropagation,
	types.PropagationSlave:    setBindPropagation,
	types.SELinuxShared:       setBindSELinux,
	types.SELinuxPrivate:      setBindSELinux,
}

func setBindPropagation(bind *types.ServiceVolumeBind, option string) {
	bind.Propagation = option
}

func setBindSELinux(bind *types.ServiceVolumeBind, option string) {
	bind.SELinux = option
}

func isBindOption(option string) bool {
	_, ok := bindOptions[option]

	return ok
}

func setBindOption(volume *types.ServiceVolumeConfig, option string) {
	if volume.Bind == nil {
		volume.Bind = &types.ServiceVolumeBind{}
	}

	bindOptions[option](volume.Bind, option)
}

func populateType(volume *types.ServiceVolumeConfig) {
	if isFilePath(volume.Source) {
		volume.Type = types.VolumeTypeBind
		if volume.Bind == nil {
			volume.Bind = &types.ServiceVolumeBind{}
		}
		// For backward compatibility with docker-compose legacy, using short notation involves
		// bind will create missing host path
		volume.Bind.CreateHostPath = true
	} else {
		volume.Type = types.VolumeTypeVolume
		if volume.Volume == nil {
			volume.Volume = &types.ServiceVolumeVolume{}
		}
	}
}

func isFilePath(source string) bool {
	if source == "" {
		return false
	}
	switch source[0] {
	case '.', '/', '~':
		return true
	}

	// windows named pipes
	if strings.HasPrefix(source, `\\`) {
		return true
	}

	first, nextIndex := utf8.DecodeRuneInString(source)
	return isWindowsDrive([]rune{first}, rune(source[nextIndex]))
}
