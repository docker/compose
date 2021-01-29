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
	"regexp"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
)

var constraintEquals = regexp.MustCompile(`([\w\.]*)\W*(==|!=)\W*([\w\.]*)`)

const (
	kubernetesOs       = "beta.kubernetes.io/os"
	kubernetesArch     = "beta.kubernetes.io/arch"
	kubernetesHostname = "kubernetes.io/hostname"
)

// node.id	Node ID	node.id == 2ivku8v2gvtg4
// node.hostname	Node hostname	node.hostname != node-2
// node.role	Node role	node.role == manager
// node.labels	user defined node labels	node.labels.security == high
// engine.labels	Docker Engine's labels	engine.labels.operatingsystem == ubuntu 14.04
func toNodeAffinity(deploy *types.DeployConfig) (*apiv1.Affinity, error) {
	constraints := []string{}
	if deploy != nil && deploy.Placement.Constraints != nil {
		constraints = deploy.Placement.Constraints
	}
	requirements := []apiv1.NodeSelectorRequirement{}
	for _, constraint := range constraints {
		matches := constraintEquals.FindStringSubmatch(constraint)
		if len(matches) == 4 {
			key := matches[1]
			operator, err := toRequirementOperator(matches[2])
			if err != nil {
				return nil, err
			}
			value := matches[3]

			switch {
			case key == constraintOs:
				requirements = append(requirements, apiv1.NodeSelectorRequirement{
					Key:      kubernetesOs,
					Operator: operator,
					Values:   []string{value},
				})
			case key == constraintArch:
				requirements = append(requirements, apiv1.NodeSelectorRequirement{
					Key:      kubernetesArch,
					Operator: operator,
					Values:   []string{value},
				})
			case key == constraintHostname:
				requirements = append(requirements, apiv1.NodeSelectorRequirement{
					Key:      kubernetesHostname,
					Operator: operator,
					Values:   []string{value},
				})
			case strings.HasPrefix(key, constraintLabelPrefix):
				requirements = append(requirements, apiv1.NodeSelectorRequirement{
					Key:      strings.TrimPrefix(key, constraintLabelPrefix),
					Operator: operator,
					Values:   []string{value},
				})
			}
		}
	}

	if !hasRequirement(requirements, kubernetesOs) {
		requirements = append(requirements, apiv1.NodeSelectorRequirement{
			Key:      kubernetesOs,
			Operator: apiv1.NodeSelectorOpIn,
			Values:   []string{"linux"},
		})
	}
	if !hasRequirement(requirements, kubernetesArch) {
		requirements = append(requirements, apiv1.NodeSelectorRequirement{
			Key:      kubernetesArch,
			Operator: apiv1.NodeSelectorOpIn,
			Values:   []string{"amd64"},
		})
	}
	return &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{
					{
						MatchExpressions: requirements,
					},
				},
			},
		},
	}, nil
}

const (
	constraintOs          = "node.platform.os"
	constraintArch        = "node.platform.arch"
	constraintHostname    = "node.hostname"
	constraintLabelPrefix = "node.labels."
)

func hasRequirement(requirements []apiv1.NodeSelectorRequirement, key string) bool {
	for _, r := range requirements {
		if r.Key == key {
			return true
		}
	}
	return false
}

func toRequirementOperator(sign string) (apiv1.NodeSelectorOperator, error) {
	switch sign {
	case "==":
		return apiv1.NodeSelectorOpIn, nil
	case "!=":
		return apiv1.NodeSelectorOpNotIn, nil
	case ">":
		return apiv1.NodeSelectorOpGt, nil
	case "<":
		return apiv1.NodeSelectorOpLt, nil
	default:
		return "", errors.Errorf("operator %s not supported", sign)
	}
}
