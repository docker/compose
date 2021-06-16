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
	"time"

	compose "github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types/container"
)

// ToMobyEnv convert into []string
func ToMobyEnv(environment compose.MappingWithEquals) []string {
	var env []string
	for k, v := range environment {
		if v == nil {
			env = append(env, k)
		} else {
			env = append(env, fmt.Sprintf("%s=%s", k, *v))
		}
	}
	return env
}

// ToMobyHealthCheck convert into container.HealthConfig
func ToMobyHealthCheck(check *compose.HealthCheckConfig) *container.HealthConfig {
	if check == nil {
		return nil
	}
	var (
		interval time.Duration
		timeout  time.Duration
		period   time.Duration
		retries  int
	)
	if check.Interval != nil {
		interval = time.Duration(*check.Interval)
	}
	if check.Timeout != nil {
		timeout = time.Duration(*check.Timeout)
	}
	if check.StartPeriod != nil {
		period = time.Duration(*check.StartPeriod)
	}
	if check.Retries != nil {
		retries = int(*check.Retries)
	}
	test := check.Test
	if check.Disable {
		test = []string{"NONE"}
	}
	return &container.HealthConfig{
		Test:        test,
		Interval:    interval,
		Timeout:     timeout,
		StartPeriod: period,
		Retries:     retries,
	}
}

// ToSeconds convert into seconds
func ToSeconds(d *compose.Duration) *int {
	if d == nil {
		return nil
	}
	s := int(time.Duration(*d).Seconds())
	return &s
}
