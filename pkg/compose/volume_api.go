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

package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
)

// volumeNeedsMountAPI tells how the engine takes a volume a service declares in
// `volumes`: as the `SOURCE:TARGET[:OPTIONS]` bind string, which is what Compose sends
// whenever it says all the volume says, or through the Mount API, when the bind string
// can't (a subpath, volume labels, a bind that doesn't create its host path, a tmpfs or
// an image...). It fails for a volume that the engine can't take either way, which are
// the ones that are said the one way and need the other:
//
//   - the SELinux relabeling is an option of the bind string only: the Mount API has no
//     place for it, so a volume that sets it and needs the Mount API can't be taken;
//   - a propagation is an option of a bind mount: the engine reads it in a bind string
//     as bind options, which a mount of type volume doesn't accept, and the Mount API of
//     a volume has none.
//
// The extensions (x-*) are not counted by compose-go: they are for the tools that read the
// model, not for the engine. The consistency is: the bind string has no place for it, so a
// volume that sets it goes through the Mount API, which sends it, even though the daemon
// accepts it and doesn't use it today. The model says it, so it is sent, and a volume that
// sets it together with the SELinux relabeling is one the engine can't take.
//
// It is a function of the volume alone, which tells whether to move it to the `mounts`
// of its container, or to refuse it, for what loads a project and wants to do it early:
// it is the one that compose-go's loader.Options.ConvertVolumesToMounts takes.
func volumeNeedsMountAPI(v types.ServiceVolumeConfig) (bool, error) {
	if v.Type != types.VolumeTypeBind && v.Bind != nil && v.Bind.Propagation != "" {
		return false, fmt.Errorf("propagation %q only applies to a bind mount, not to a %s", v.Bind.Propagation, v.Type)
	}
	if _, ok := v.ShortSyntax(); ok {
		return false, nil
	}
	if v.Bind != nil && v.Bind.SELinux != "" {
		return false, fmt.Errorf("selinux %q can't be applied: the volume needs the Mount API, "+
			"which has no place for the SELinux relabeling, and the bind string can't say all the volume says", v.Bind.SELinux)
	}
	return true, nil
}
