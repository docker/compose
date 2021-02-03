// +build kube

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

package resources

import (
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/types"

	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
)

/* FIXME
func TestToPodWithPlacement(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: redis:alpine
    deploy:
      placement:
        constraints:
          - node.platform.os == linux
          - node.platform.arch == amd64
          - node.hostname == node01
          - node.labels.label1 == value1
          - node.labels.label2.subpath != value2
`)

	expectedRequirements := []apiv1.NodeSelectorRequirement{
		{Key: "beta.kubernetes.io/os", Operator: apiv1.NodeSelectorOpIn, Values: []string{"linux"}},
		{Key: "beta.kubernetes.io/arch", Operator: apiv1.NodeSelectorOpIn, Values: []string{"amd64"}},
		{Key: "kubernetes.io/hostname", Operator: apiv1.NodeSelectorOpIn, Values: []string{"node01"}},
		{Key: "label1", Operator: apiv1.NodeSelectorOpIn, Values: []string{"value1"}},
		{Key: "label2.subpath", Operator: apiv1.NodeSelectorOpNotIn, Values: []string{"value2"}},
	}

	requirements := podTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions

	sort.Slice(expectedRequirements, func(i, j int) bool { return expectedRequirements[i].Key < expectedRequirements[j].Key })
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].Key < requirements[j].Key })

	assert.EqualValues(t, expectedRequirements, requirements)
}
*/

type keyValue struct {
	key   string
	value string
}

func kv(key, value string) keyValue {
	return keyValue{key: key, value: value}
}

func makeExpectedAffinity(kvs ...keyValue) *apiv1.Affinity {

	var matchExpressions []apiv1.NodeSelectorRequirement
	for _, kv := range kvs {
		matchExpressions = append(
			matchExpressions,
			apiv1.NodeSelectorRequirement{
				Key:      kv.key,
				Operator: apiv1.NodeSelectorOpIn,
				Values:   []string{kv.value},
			},
		)
	}
	return &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{
					{
						MatchExpressions: matchExpressions,
					},
				},
			},
		},
	}
}

func TestNodeAfinity(t *testing.T) {
	cases := []struct {
		name     string
		source   []string
		expected *apiv1.Affinity
	}{
		{
			name: "nil",
			expected: makeExpectedAffinity(
				kv(kubernetesOs, "linux"),
				kv(kubernetesArch, "amd64"),
			),
		},
		{
			name:   "hostname",
			source: []string{"node.hostname == test"},
			expected: makeExpectedAffinity(
				kv(kubernetesHostname, "test"),
				kv(kubernetesOs, "linux"),
				kv(kubernetesArch, "amd64"),
			),
		},
		{
			name:   "os",
			source: []string{"node.platform.os == windows"},
			expected: makeExpectedAffinity(
				kv(kubernetesOs, "windows"),
				kv(kubernetesArch, "amd64"),
			),
		},
		{
			name:   "arch",
			source: []string{"node.platform.arch == arm64"},
			expected: makeExpectedAffinity(
				kv(kubernetesArch, "arm64"),
				kv(kubernetesOs, "linux"),
			),
		},
		{
			name:   "custom-labels",
			source: []string{"node.platform.os == windows", "node.platform.arch == arm64"},
			expected: makeExpectedAffinity(
				kv(kubernetesArch, "arm64"),
				kv(kubernetesOs, "windows"),
			),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := toNodeAffinity(&types.DeployConfig{
				Placement: types.Placement{
					Constraints: c.source,
				},
			})
			assert.NoError(t, err)
			assert.True(t, nodeAffinityMatch(c.expected, result))
		})
	}
}

func nodeSelectorRequirementsToMap(source []apiv1.NodeSelectorRequirement, result map[string]apiv1.NodeSelectorRequirement) {
	for _, t := range source {
		result[t.Key] = t
	}
}

func nodeAffinityMatch(expected, actual *apiv1.Affinity) bool {
	expectedTerms := expected.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	actualTerms := actual.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	expectedExpressions := make(map[string]apiv1.NodeSelectorRequirement)
	expectedFields := make(map[string]apiv1.NodeSelectorRequirement)
	actualExpressions := make(map[string]apiv1.NodeSelectorRequirement)
	actualFields := make(map[string]apiv1.NodeSelectorRequirement)
	for _, v := range expectedTerms {
		nodeSelectorRequirementsToMap(v.MatchExpressions, expectedExpressions)
		nodeSelectorRequirementsToMap(v.MatchFields, expectedFields)
	}
	for _, v := range actualTerms {
		nodeSelectorRequirementsToMap(v.MatchExpressions, actualExpressions)
		nodeSelectorRequirementsToMap(v.MatchFields, actualFields)
	}
	return reflect.DeepEqual(expectedExpressions, actualExpressions) && reflect.DeepEqual(expectedFields, actualFields)
}
