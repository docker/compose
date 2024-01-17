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
	"context"
	"errors"
	"fmt"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/versions"
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
func (s *composeService) ToMobyHealthCheck(ctx context.Context, check *compose.HealthCheckConfig) (*container.HealthConfig, error) {
	if check == nil {
		return nil, nil
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
	var startInterval time.Duration
	if check.StartInterval != nil {
		version, err := s.RuntimeVersion(ctx)
		if err != nil {
			return nil, err
		}
		if versions.LessThan(version, "1.44") {
			return nil, errors.New("can't set healthcheck.start_interval as feature require Docker Engine v25 or later")
		} else {
			startInterval = time.Duration(*check.StartInterval)
		}
	}
	return &container.HealthConfig{
		Test:          test,
		Interval:      interval,
		Timeout:       timeout,
		StartPeriod:   period,
		StartInterval: startInterval,
		Retries:       retries,
	}, nil
}

// ToSeconds convert into seconds
func ToSeconds(d *compose.Duration) *int {
	if d == nil {
		return nil
	}
	s := int(time.Duration(*d).Seconds())
	return &s
}
