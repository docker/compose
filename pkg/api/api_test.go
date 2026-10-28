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

package api

import (
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestPortPublisherString(t *testing.T) {
	tests := []struct {
		name string
		pub  PortPublisher
		want string
	}{
		{"ipv4", PortPublisher{URL: "0.0.0.0", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"}, "80/tcp -> 0.0.0.0:8080"},
		{"ipv6", PortPublisher{URL: "::", TargetPort: 5060, PublishedPort: 32769, Protocol: "udp"}, "5060/udp -> [::]:32769"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.pub.String(), tt.want)
		})
	}
}

func TestRunOptionsEnvironmentMap(t *testing.T) {
	opts := RunOptions{
		Environment: []string{
			"FOO=BAR",
			"ZOT=",
			"QIX",
		},
	}
	env := types.NewMappingWithEquals(opts.Environment)
	assert.Equal(t, *env["FOO"], "BAR")
	assert.Equal(t, *env["ZOT"], "")
	assert.Check(t, env["QIX"] == nil)
}

// Callers written before ExecOptions existed pass a RunOptions to Exec.
var (
	_ ExecOptionsProvider = RunOptions{}
	_ ExecOptionsProvider = ExecOptions{}
)

func TestRunOptionsToExecOptions(t *testing.T) {
	run := RunOptions{
		Service:     "web",
		Index:       2,
		Command:     []string{"ls", "-l"},
		Environment: []string{"FOO=BAR"},
		Tty:         true,
		Interactive: true,
		Detach:      true,
		User:        "nobody",
		Privileged:  true,
		WorkingDir:  "/srv",
		Name:        "ignored",
	}
	assert.DeepEqual(t, run.ToExecOptions(), ExecOptions{
		Service:     "web",
		Index:       2,
		Command:     []string{"ls", "-l"},
		Environment: []string{"FOO=BAR"},
		Tty:         true,
		Interactive: true,
		Detach:      true,
		User:        "nobody",
		Privileged:  true,
		WorkingDir:  "/srv",
	})
}

// Every field of ExecOptions must be copied from the RunOptions field of the
// same name, so that a field added to ExecOptions is not silently dropped.
func TestRunOptionsToExecOptionsCoversAllFields(t *testing.T) {
	run := reflect.New(reflect.TypeOf(RunOptions{})).Elem()
	exec := reflect.TypeOf(ExecOptions{})
	for i := 0; i < exec.NumField(); i++ {
		f := exec.Field(i)
		rf := run.FieldByName(f.Name)
		assert.Assert(t, rf.IsValid(), "RunOptions has no field %s", f.Name)
		assert.Equal(t, rf.Type(), f.Type, "field %s", f.Name)
		switch f.Type.Kind() {
		case reflect.String:
			rf.SetString("x")
		case reflect.Int:
			rf.SetInt(1)
		case reflect.Bool:
			rf.SetBool(true)
		case reflect.Slice:
			rf.Set(reflect.ValueOf([]string{"x"}))
		default:
			t.Fatalf("unhandled kind %s for %s", f.Type.Kind(), f.Name)
		}
	}
	got := reflect.ValueOf(run.Interface().(RunOptions).ToExecOptions())
	for i := 0; i < exec.NumField(); i++ {
		assert.Assert(t, !got.Field(i).IsZero(), "field %s is not copied by ToExecOptions", exec.Field(i).Name)
	}
}

func TestGetDependentImages(t *testing.T) {
	const projectName = "demo"
	tests := []struct {
		name     string
		service  types.ServiceConfig
		expected []string
	}{
		{
			name:     "no hooks",
			service:  types.ServiceConfig{ContainerSpec: types.ContainerSpec{Image: "alpine:3.20"}},
			expected: nil,
		},
		{
			name: "pre_start hook with explicit image",
			service: types.ServiceConfig{
				PreStart: []types.PreStartHook{
					{ContainerSpec: types.ContainerSpec{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "init"}}},
				}, ContainerSpec: types.ContainerSpec{Image: "alpine:3.20"},
			},
			expected: []string{"alpine:3.19"},
		},
		{
			name: "pre_start hook without image is ignored",
			service: types.ServiceConfig{
				PreStart: []types.PreStartHook{
					{ContainerSpec: types.ContainerSpec{Image: "busybox", Command: types.ShellCommand{"echo", "a"}}},
					{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"echo", "b"}}},
				}, ContainerSpec: types.ContainerSpec{Image: "alpine:3.20"},
			},
			expected: []string{"busybox"},
		},
		{
			name: "pre_start hook reusing the service image is ignored",
			service: types.ServiceConfig{
				PreStart: []types.PreStartHook{
					{ContainerSpec: types.ContainerSpec{Image: "alpine:3.20", Command: types.ShellCommand{"echo", "same"}}},
					{ContainerSpec: types.ContainerSpec{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "other"}}},
				}, ContainerSpec: types.ContainerSpec{Image: "alpine:3.20"},
			},
			expected: []string{"alpine:3.19"},
		},
		{
			name: "pre_start hook reusing the default (build) image name is ignored",
			service: types.ServiceConfig{
				Name: "web",

				PreStart: []types.PreStartHook{
					{ContainerSpec: types.ContainerSpec{Image: "demo-web", Command: types.ShellCommand{"echo", "same"}}},
				}, WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
			},
			expected: nil,
		},
		{
			// exec hooks carry no image at all since the container spec
			// layering: nothing to collect, by construction
			name: "post_start and pre_stop hooks are not collected",
			service: types.ServiceConfig{
				PostStart:     []types.ServiceHook{{Command: types.ShellCommand{"echo"}}},
				PreStop:       []types.ServiceHook{{Command: types.ShellCommand{"echo"}}},
				ContainerSpec: types.ContainerSpec{Image: "alpine:3.20"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, GetDependentImages(tt.service, projectName), tt.expected)
		})
	}
}
