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
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name   string
		up     upOptions
		create createOptions
		// wantErr is the exact error message, empty when the combination is valid
		wantErr string
	}{
		{name: "defaults are valid"},
		{
			name:    "negative wait timeout",
			up:      upOptions{waitTimeout: -1},
			wantErr: "--wait-timeout must be a non-negative integer",
		},
		{
			name:    "stop and fail cascades",
			up:      upOptions{cascadeStop: true, cascadeFail: true},
			wantErr: "--abort-on-container-failure cannot be combined with --abort-on-container-exit",
		},
		{
			name:    "wait with abort-on-container-exit",
			up:      upOptions{wait: true, cascadeStop: true},
			wantErr: "--wait cannot be combined with --abort-on-container-exit, --attach or --attach-dependencies",
		},
		{
			name:    "wait with attach",
			up:      upOptions{wait: true, attach: []string{"web"}},
			wantErr: "--wait cannot be combined with --abort-on-container-exit, --attach or --attach-dependencies",
		},
		{
			name:    "wait with attach-dependencies",
			up:      upOptions{wait: true, attachDependencies: true},
			wantErr: "--wait cannot be combined with --abort-on-container-exit, --attach or --attach-dependencies",
		},
		{
			name:    "build and no-build",
			create:  createOptions{Build: true, noBuild: true},
			wantErr: "--build and --no-build are incompatible",
		},
		{
			name:    "detach with attach-dependencies",
			up:      upOptions{Detach: true, attachDependencies: true},
			wantErr: "--detach cannot be combined with --abort-on-container-exit, --abort-on-container-failure, --attach, --attach-dependencies or --watch",
		},
		{
			name:    "detach with abort-on-container-failure",
			up:      upOptions{Detach: true, cascadeFail: true},
			wantErr: "--detach cannot be combined with --abort-on-container-exit, --abort-on-container-failure, --attach, --attach-dependencies or --watch",
		},
		{
			name:    "detach with watch",
			up:      upOptions{Detach: true, watch: true},
			wantErr: "--detach cannot be combined with --abort-on-container-exit, --abort-on-container-failure, --attach, --attach-dependencies or --watch",
		},
		{
			name:    "no-inherit and no-recreate",
			create:  createOptions{noInherit: true, noRecreate: true},
			wantErr: "--no-recreate and --renew-anon-volumes are incompatible",
		},
		{
			name:    "force-recreate and no-recreate",
			create:  createOptions{forceRecreate: true, noRecreate: true},
			wantErr: "--force-recreate and --no-recreate are incompatible",
		},
		{
			name:    "always-recreate-deps and no-recreate",
			create:  createOptions{recreateDeps: true, noRecreate: true},
			wantErr: "--always-recreate-deps and --no-recreate are incompatible",
		},
		{
			name:    "no-build and watch",
			create:  createOptions{noBuild: true},
			up:      upOptions{watch: true},
			wantErr: "--no-build and --watch are incompatible",
		},
		{
			name:   "watch alone is valid",
			up:     upOptions{watch: true},
			create: createOptions{Build: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFlags(&tt.up, &tt.create)
			if tt.wantErr == "" {
				assert.NilError(t, err)
				return
			}
			assert.Error(t, err, tt.wantErr)
		})
	}
}

func TestValidateFlagsImplications(t *testing.T) {
	t.Run("exit-code-from implies abort-on-container-exit", func(t *testing.T) {
		up := upOptions{exitCodeFrom: "web"}
		assert.NilError(t, validateFlags(&up, &createOptions{}))
		assert.Assert(t, up.cascadeStop)
		assert.Equal(t, up.OnExit(), api.CascadeStop)
	})
	t.Run("exit-code-from keeps abort-on-container-failure", func(t *testing.T) {
		up := upOptions{exitCodeFrom: "web", cascadeFail: true}
		assert.NilError(t, validateFlags(&up, &createOptions{}))
		assert.Assert(t, !up.cascadeStop)
		assert.Equal(t, up.OnExit(), api.CascadeFail)
	})
	t.Run("wait implies detach", func(t *testing.T) {
		up := upOptions{wait: true}
		assert.NilError(t, validateFlags(&up, &createOptions{}))
		assert.Assert(t, up.Detach)
	})
	t.Run("exit-code-from conflicts with wait through the implied cascade", func(t *testing.T) {
		up := upOptions{exitCodeFrom: "web", wait: true}
		err := validateFlags(&up, &createOptions{})
		assert.Error(t, err, "--wait cannot be combined with --abort-on-container-exit, --attach or --attach-dependencies")
	})
}

func TestUpOptionsApply(t *testing.T) {
	project := func() *types.Project {
		return &types.Project{
			Services: types.Services{
				"web": {Name: "web", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionStarted, Required: true},
				}}},
				"db": {Name: "db"},
			},
		}
	}

	tests := []struct {
		name         string
		opts         upOptions
		services     []string
		wantServices []string
		wantErr      string
	}{
		{
			name:         "keeps dependencies by default",
			services:     []string{"web"},
			wantServices: []string{"db", "web"},
		},
		{
			name:         "no-deps drops dependencies",
			opts:         upOptions{noDeps: true},
			services:     []string{"web"},
			wantServices: []string{"web"},
		},
		{
			name:         "exit-code-from an existing service",
			opts:         upOptions{exitCodeFrom: "db"},
			wantServices: []string{"db", "web"},
		},
		{
			name:    "exit-code-from an unknown service",
			opts:    upOptions{exitCodeFrom: "nope"},
			wantErr: `no such service: nope`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.opts.apply(project(), tt.services)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got.ServiceNames(), tt.wantServices)
		})
	}
}

func TestCreateOptionsStrategies(t *testing.T) {
	tests := []struct {
		name     string
		opts     createOptions
		services string
		deps     string
	}{
		{name: "default", services: api.RecreateDiverged, deps: api.RecreateDiverged},
		{name: "no-recreate", opts: createOptions{noRecreate: true}, services: api.RecreateNever, deps: api.RecreateNever},
		{name: "force-recreate", opts: createOptions{forceRecreate: true}, services: api.RecreateForce, deps: api.RecreateDiverged},
		{name: "renew-anon-volumes", opts: createOptions{noInherit: true}, services: api.RecreateForce, deps: api.RecreateDiverged},
		{name: "always-recreate-deps", opts: createOptions{recreateDeps: true}, services: api.RecreateDiverged, deps: api.RecreateForce},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.opts.recreateStrategy(), tt.services)
			assert.Equal(t, tt.opts.dependenciesRecreateStrategy(), tt.deps)
		})
	}
}

func TestCreateOptionsGetTimeout(t *testing.T) {
	assert.Assert(t, createOptions{timeout: 5}.GetTimeout() == nil, "timeout must be left to the service when the flag was not set")

	got := createOptions{timeout: 5, timeChanged: true}.GetTimeout()
	assert.Assert(t, got != nil)
	assert.Equal(t, *got, 5*time.Second)

	zero := createOptions{timeChanged: true}.GetTimeout()
	assert.Assert(t, zero != nil)
	assert.Equal(t, *zero, time.Duration(0))
}

func TestCreateOptionsApply(t *testing.T) {
	project := func() *types.Project {
		return &types.Project{
			Services: types.Services{
				"built": {
					Name:          "built",
					ContainerSpec: types.ContainerSpec{PullPolicy: types.PullPolicyMissing},
					WorkloadSpec:  types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
				},
				"pulled": {Name: "pulled", ContainerSpec: types.ContainerSpec{Image: "nginx"}},
			},
		}
	}

	tests := []struct {
		name       string
		opts       createOptions
		wantPolicy map[string]string
		wantErr    string
	}{
		{
			name:       "no flag keeps compose file policies",
			wantPolicy: map[string]string{"built": types.PullPolicyMissing, "pulled": ""},
		},
		{
			name:       "pull applies to every service",
			opts:       createOptions{Pull: types.PullPolicyAlways, pullChanged: true},
			wantPolicy: map[string]string{"built": types.PullPolicyAlways, "pulled": types.PullPolicyAlways},
		},
		{
			name:       "pull value is ignored when the flag was not set",
			opts:       createOptions{Pull: "bogus"},
			wantPolicy: map[string]string{"built": types.PullPolicyMissing, "pulled": ""},
		},
		{
			name:    "invalid pull policy",
			opts:    createOptions{Pull: "sometimes", pullChanged: true},
			wantErr: `invalid --pull option "sometimes"`,
		},
		{
			name:       "build forces build on services having a build section only",
			opts:       createOptions{Build: true},
			wantPolicy: map[string]string{"built": types.PullPolicyBuild, "pulled": ""},
		},
		{
			name:    "malformed scale",
			opts:    createOptions{scale: []string{"built"}},
			wantErr: `invalid --scale option "built". Should be SERVICE=NUM`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := project()
			err := tt.opts.Apply(p)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			for name, want := range tt.wantPolicy {
				assert.Equal(t, p.Services[name].PullPolicy, want, name)
			}
		})
	}
}

func TestApplyScaleOptsErrors(t *testing.T) {
	tests := []struct {
		name    string
		scale   []string
		wantErr string
	}{
		{name: "missing separator", scale: []string{"foo"}, wantErr: `invalid --scale option "foo". Should be SERVICE=NUM`},
		{name: "missing value", scale: []string{"foo="}, wantErr: `invalid --scale option "foo=". Should be SERVICE=NUM`},
		{name: "not a number", scale: []string{"foo=many"}, wantErr: `strconv.Atoi: parsing "many": invalid syntax`},
		{name: "unknown service", scale: []string{"nope=2"}, wantErr: "no such service: nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &types.Project{Services: types.Services{"foo": {Name: "foo"}}}
			assert.ErrorContains(t, applyScaleOpts(p, tt.scale), tt.wantErr)
		})
	}
}
