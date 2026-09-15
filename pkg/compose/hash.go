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
	"bytes"
	"encoding/json"
	"sort"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/opencontainers/go-digest"
)

// serviceHashKeyOrder freezes the top-level JSON key order of the service
// config-hash. Hashing json.Marshal of the struct directly would couple every
// recorded hash to the DECLARATION ORDER of compose-go's fields —
// encoding/json emits struct fields in that order, and flattens embedded
// structs at their embedding position — so any compose-go refactoring moving
// a field (or grouping fields into embedded specs) would change the bytes,
// and with them the hash, of configurations that did not change at all:
// every container recreated on the first `up` after an upgrade.
//
// This list pins the byte layout to the historical form instead, generated
// by reflection over compose-go v2.15.1-0.20260908103050 (the last layout
// every released hash was computed from), so existing container stamps stay
// valid verbatim: no migration, no recreation. A root attribute missing from
// the list (added to compose-go later) is emitted after the listed ones, in
// sorted order — deterministic, and thanks to omitempty only configurations
// using the new attribute see their hash move, exactly like a field addition
// always did. Nested objects keep the struct marshal of their own types; a
// reorder inside one of them would still move hashes — the golden tests
// exist to turn that into a reviewed decision instead of a side effect.
var serviceHashKeyOrder = []string{
	"profiles",
	"annotations",
	"attach",
	"build",
	"develop",
	"blkio_config",
	"cap_add",
	"cap_drop",
	"cgroup_parent",
	"cgroup",
	"cpu_count",
	"cpu_percent",
	"cpu_period",
	"cpu_quota",
	"cpu_rt_period",
	"cpu_rt_runtime",
	"cpus",
	"cpuset",
	"cpu_shares",
	"command",
	"configs",
	"container_name",
	"credential_spec",
	"depends_on",
	"deploy",
	"device_cgroup_rules",
	"devices",
	"dns",
	"dns_opt",
	"dns_search",
	"dockerfile",
	"domainname",
	"entrypoint",
	"provider",
	"environment",
	"env_file",
	"expose",
	"extends",
	"external_links",
	"extra_hosts",
	"group_add",
	"gpus",
	"hostname",
	"healthcheck",
	"image",
	"init",
	"ipc",
	"isolation",
	"labels",
	"label_file",
	"links",
	"logging",
	"log_driver",
	"log_opt",
	"mem_limit",
	"mem_reservation",
	"memswap_limit",
	"mem_swappiness",
	"mac_address",
	"models",
	"net",
	"network_mode",
	"networks",
	"oom_kill_disable",
	"oom_score_adj",
	"pid",
	"pids_limit",
	"platform",
	"ports",
	"privileged",
	"pull_policy",
	"read_only",
	"restart",
	"runtime",
	"scale",
	"secrets",
	"security_opt",
	"shm_size",
	"stdin_open",
	"stop_grace_period",
	"stop_signal",
	"storage_opt",
	"sysctls",
	"tmpfs",
	"tty",
	"ulimits",
	"use_api_socket",
	"user",
	"userns_mode",
	"uts",
	"volume_driver",
	"volumes",
	"volumes_from",
	"working_dir",
	"pre_start",
	"post_start",
	"pre_stop",
}

// ServiceHash computes the configuration hash for a service.
func ServiceHash(o types.ServiceConfig) (string, error) {
	// remove the Build config when generating the service hash
	o.Build = nil
	o.PullPolicy = ""
	o.Scale = nil
	if o.Deploy != nil {
		deploy := *o.Deploy
		deploy.Replicas = nil
		o.Deploy = &deploy
	}
	o.DependsOn = nil
	o.Profiles = nil

	raw, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	pinned, err := pinRootKeyOrder(raw, serviceHashKeyOrder)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(pinned).Encoded(), nil
}

// pinRootKeyOrder re-emits a JSON object with its top-level keys in the
// given order (values kept byte-verbatim), keys absent from the list
// appended in sorted order. For an object whose keys all follow the list,
// the output is byte-identical to the input.
func pinRootKeyOrder(raw []byte, order []string) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	write := func(key string, val json.RawMessage) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		name, _ := json.Marshal(key)
		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(val)
	}
	for _, key := range order {
		if val, ok := root[key]; ok {
			write(key, val)
			delete(root, key)
		}
	}
	rest := make([]string, 0, len(root))
	for key := range root {
		rest = append(rest, key)
	}
	sort.Strings(rest)
	for _, key := range rest {
		write(key, root[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// NetworkHash computes the configuration hash for a network.
func NetworkHash(o *types.NetworkConfig) (string, error) {
	raw, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(raw).Encoded(), nil
}

// VolumeHash computes the configuration hash for a volume.
func VolumeHash(o types.VolumeConfig) (string, error) {
	if o.Driver == "" { // (TODO: jhrotko) This probably should be fixed in compose-go
		o.Driver = "local"
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(raw).Encoded(), nil
}
