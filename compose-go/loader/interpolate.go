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
	"strconv"
	"strings"

	interp "github.com/compose-spec/compose-go/interpolation"
	"github.com/pkg/errors"
)

var interpolateTypeCastMapping = map[interp.Path]interp.Cast{
	servicePath("configs", interp.PathMatchList, "mode"):             toInt,
	servicePath("cpu_count"):                                         toInt64,
	servicePath("cpu_percent"):                                       toFloat,
	servicePath("cpu_period"):                                        toInt64,
	servicePath("cpu_quota"):                                         toInt64,
	servicePath("cpu_rt_period"):                                     toInt64,
	servicePath("cpu_rt_runtime"):                                    toInt64,
	servicePath("cpus"):                                              toFloat32,
	servicePath("cpu_shares"):                                        toInt64,
	servicePath("init"):                                              toBoolean,
	servicePath("deploy", "replicas"):                                toInt,
	servicePath("deploy", "update_config", "parallelism"):            toInt,
	servicePath("deploy", "update_config", "max_failure_ratio"):      toFloat,
	servicePath("deploy", "rollback_config", "parallelism"):          toInt,
	servicePath("deploy", "rollback_config", "max_failure_ratio"):    toFloat,
	servicePath("deploy", "restart_policy", "max_attempts"):          toInt,
	servicePath("deploy", "placement", "max_replicas_per_node"):      toInt,
	servicePath("healthcheck", "retries"):                            toInt,
	servicePath("healthcheck", "disable"):                            toBoolean,
	servicePath("mem_limit"):                                         toUnitBytes,
	servicePath("mem_reservation"):                                   toUnitBytes,
	servicePath("memswap_limit"):                                     toUnitBytes,
	servicePath("mem_swappiness"):                                    toUnitBytes,
	servicePath("oom_kill_disable"):                                  toBoolean,
	servicePath("oom_score_adj"):                                     toInt64,
	servicePath("pids_limit"):                                        toInt64,
	servicePath("ports", interp.PathMatchList, "target"):             toInt,
	servicePath("privileged"):                                        toBoolean,
	servicePath("read_only"):                                         toBoolean,
	servicePath("scale"):                                             toInt,
	servicePath("secrets", interp.PathMatchList, "mode"):             toInt,
	servicePath("shm_size"):                                          toUnitBytes,
	servicePath("stdin_open"):                                        toBoolean,
	servicePath("stop_grace_period"):                                 toDuration,
	servicePath("tty"):                                               toBoolean,
	servicePath("ulimits", interp.PathMatchAll):                      toInt,
	servicePath("ulimits", interp.PathMatchAll, "hard"):              toInt,
	servicePath("ulimits", interp.PathMatchAll, "soft"):              toInt,
	servicePath("volumes", interp.PathMatchList, "read_only"):        toBoolean,
	servicePath("volumes", interp.PathMatchList, "volume", "nocopy"): toBoolean,
	servicePath("volumes", interp.PathMatchList, "tmpfs", "size"):    toUnitBytes,
	iPath("networks", interp.PathMatchAll, "external"):               toBoolean,
	iPath("networks", interp.PathMatchAll, "internal"):               toBoolean,
	iPath("networks", interp.PathMatchAll, "attachable"):             toBoolean,
	iPath("networks", interp.PathMatchAll, "enable_ipv6"):            toBoolean,
	iPath("volumes", interp.PathMatchAll, "external"):                toBoolean,
	iPath("secrets", interp.PathMatchAll, "external"):                toBoolean,
	iPath("configs", interp.PathMatchAll, "external"):                toBoolean,
}

func iPath(parts ...string) interp.Path {
	return interp.NewPath(parts...)
}

func servicePath(parts ...string) interp.Path {
	return iPath(append([]string{"services", interp.PathMatchAll}, parts...)...)
}

func toInt(value string) (interface{}, error) {
	return strconv.Atoi(value)
}

func toInt64(value string) (interface{}, error) {
	return strconv.ParseInt(value, 10, 64)
}

func toUnitBytes(value string) (interface{}, error) {
	return transformSize(value)
}

func toDuration(value string) (interface{}, error) {
	return transformStringToDuration(value)
}

func toFloat(value string) (interface{}, error) {
	return strconv.ParseFloat(value, 64)
}

func toFloat32(value string) (interface{}, error) {
	f, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return nil, err
	}
	return float32(f), nil
}

// should match http://yaml.org/type/bool.html
func toBoolean(value string) (interface{}, error) {
	switch strings.ToLower(value) {
	case "y", "yes", "true", "on":
		return true, nil
	case "n", "no", "false", "off":
		return false, nil
	default:
		return nil, errors.Errorf("invalid boolean: %s", value)
	}
}
