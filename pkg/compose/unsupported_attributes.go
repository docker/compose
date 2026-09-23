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
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/schema"
	"github.com/compose-spec/compose-go/v2/tree"

	"github.com/docker/compose/v5/pkg/api"
)

// genericUnsupportedReason is used for a finding that matches neither
// presenceUnsupportedAttributes nor valueConditionalAttributes: an attribute
// the compose-spec schema gained after this file was last updated, caught
// only because it's absent from supportedAttributePaths. This is the whole
// point of allowlisting instead of denylisting — see that var's comment.
const genericUnsupportedReason = "not honored by this runtime outside Swarm mode"

// presenceUnsupportedAttributes lists, by path, every schema-declared
// attribute this runtime never honors outside Swarm mode regardless of its
// value — presence alone is the finding. Their paths are removed from
// schema.AttributePaths() (the full compose-spec inventory) to build
// supportedAttributePaths, the allowlist handed to compose-go via
// loader.WithSupportedAttributes: any OTHER attribute the specification
// adds later is reported too, with genericUnsupportedReason, until it's
// deliberately added here (or to valueConditionalAttributes) with a
// considered message. This is what makes the check fail-closed instead of
// silently missing whatever gets added to the spec next — the exact gap
// docker/compose#13150 was filed for.
var presenceUnsupportedAttributes = map[tree.Path]string{
	tree.NewPath("services", "*", "deploy", "mode"):            "deploy.mode is only honored in Swarm mode",
	tree.NewPath("services", "*", "deploy", "labels"):          "deploy.labels are only applied to Swarm services",
	tree.NewPath("services", "*", "deploy", "update_config"):   "deploy.update_config only applies to rolling updates in Swarm mode",
	tree.NewPath("services", "*", "deploy", "rollback_config"): "deploy.rollback_config only applies to rolling updates in Swarm mode",
	tree.NewPath("services", "*", "deploy", "endpoint_mode"):   "deploy.endpoint_mode only applies to Swarm's routing mesh",
	tree.NewPath("services", "*", "deploy", "placement"):       "deploy.placement is only honored by the Swarm scheduler",
	tree.NewPath("services", "*", "credential_spec"):           "credential_spec is only supported when running Windows containers under Swarm",
	tree.NewPath("configs", "*", "labels"):                     "configs[].labels are only applied to Swarm config objects",
	tree.NewPath("secrets", "*", "driver_opts"):                "secrets[].driver_opts is only honored by Swarm secret drivers",
	tree.NewPath("secrets", "*", "labels"):                     "secrets[].labels are only applied to Swarm secret objects",
}

// supportedAttributePaths is the allowlist handed to compose-go: every
// compose-spec attribute path except the ones this runtime deliberately
// never honors (presenceUnsupportedAttributes) and their descendants.
// Several removed paths (deploy.update_config, deploy.rollback_config,
// deploy.placement, credential_spec) are themselves objects with their own
// schema-declared sub-fields: schema.AttributePaths() lists those
// sub-fields as separate entries too, so removing only the exact parent
// path would leave them in Supported — making compose-go's walk see a
// supported descendant below the removed node (Matcher.MayContain) and
// descend into it instead of reporting the parent as a whole, silently
// undoing the removal. Stripping every path that has a removed one as a
// prefix — not just an exact match — is what makes the parent-level removal
// actually take.
//
// Computed lazily (schema.AttributePaths() parses the embedded compose-spec
// schema on first call) rather than at package init: most compose commands
// never set OnUnsupportedAttribute at all (e.g. shell completion, or any
// command that skips the warning — see unsupportedAttributesLoadOption's
// only caller), so paying this cost eagerly for every invocation would be
// wasted on them.
var supportedAttributePaths = sync.OnceValue(buildSupportedAttributePaths)

func buildSupportedAttributePaths() []tree.Path {
	all := schema.AttributePaths()
	supported := make([]tree.Path, 0, len(all))
	for _, path := range all {
		if isUnderAnyOf(path, presenceUnsupportedAttributes) {
			continue
		}
		supported = append(supported, path)
	}
	return supported
}

// isUnderAnyOf reports whether path equals, or is a descendant of, one of
// removed's keys: truncated to the prefix's own length, path matches it
// under tree.Path's usual "*"/"[]" wildcard rules.
func isUnderAnyOf(path tree.Path, removed map[tree.Path]string) bool {
	pathParts := path.Parts()
	for prefix := range removed {
		prefixParts := prefix.Parts()
		if len(pathParts) < len(prefixParts) {
			continue
		}
		if tree.NewPath(pathParts[:len(prefixParts)]...).Matches(prefix) {
			return true
		}
	}
	return false
}

// valueConditionalAttributes lists the checks that need a value predicate,
// not mere presence: compose-go's loader defaults each of these paths to a
// common, supported value on virtually every occurrence (ports[].mode to
// "ingress", volumes[].type to "bind"/"volume", every depends_on entry has
// some condition), so presence alone would fire on nearly every service —
// only a specific value is unsupported. They can never be derived from
// schema.AttributePaths() (compose-go's allowlist only knows about presence,
// not values), so they stay hand-written regardless of how
// presenceUnsupportedAttributes evolves.
//
// ports[].mode, volumes[].type and the two file-reference checks target the
// whole node (the map compose-go hands back for that path), not a scalar
// leaf: identifying which port/volume/reference triggered the finding needs
// a sibling field (target/protocol, source) only visible from that node — a
// list index collapses to the literal token "[]" in the reported Path,
// losing identity, unlike a map key (a service or dependency name), which
// stays.
//
// depends_on.*.condition has no entry here even though it's the same kind
// of "only some values are supported" case: the compose-spec schema itself
// declares condition as a closed enum of the 3 values this runtime
// understands, so schema.Validate — which runs before this check, on every
// load — already rejects anything else. There is no unsupported value left
// for a real compose file to reach this check with (see
// TestDependsOnUnknownConditionIsRejectedBySchema). waitDependency's own
// runtime fallback (service_containers.go) still needs to warn about an
// unrecognized condition, for the two situations that never go through
// schema validation at all: a project rebuilt from container labels, or an
// older/newer Compose version's output.
var valueConditionalAttributes = []struct {
	Pattern tree.Path
	Detect  func(value any) bool
	// Report returns the api.UnsupportedAttribute(s) for a match — Service
	// is always left blank here; toAPIUnsupportedAttributes fills it in
	// from the finding's Path, which Report doesn't have direct access to.
	Report func(finding loader.UnsupportedAttribute) []api.UnsupportedAttribute
}{
	{
		Pattern: tree.NewPath("services", "*", "ports", "[]"),
		Detect: func(v any) bool {
			port, _ := v.(map[string]any)
			return port["mode"] == "host"
		},
		Report: func(finding loader.UnsupportedAttribute) []api.UnsupportedAttribute {
			port, _ := finding.Value.(map[string]any)
			path := fmt.Sprintf("ports[%v/%v].mode", port["target"], port["protocol"])
			return []api.UnsupportedAttribute{{Path: path, Reason: "ports[].mode: host is only honored by the Swarm routing mesh"}}
		},
	},
	{
		Pattern: tree.NewPath("services", "*", "volumes", "[]"),
		Detect: func(v any) bool {
			volume, _ := v.(map[string]any)
			return volume["type"] == "cluster"
		},
		Report: func(finding loader.UnsupportedAttribute) []api.UnsupportedAttribute {
			volume, _ := finding.Value.(map[string]any)
			path := fmt.Sprintf("volumes[%v].type", volume["source"])
			return []api.UnsupportedAttribute{{Path: path, Reason: "volumes[].type: cluster (CSI) volumes are only supported in Swarm mode"}}
		},
	},
	{
		Pattern: tree.NewPath("services", "*", "configs", "[]"),
		Detect:  hasFileReferenceOverride,
		Report:  fileReferenceReport("configs"),
	},
	{
		Pattern: tree.NewPath("services", "*", "secrets", "[]"),
		Detect:  hasFileReferenceOverride,
		Report:  fileReferenceReport("secrets"),
	},
}

func hasFileReferenceOverride(v any) bool {
	ref, _ := v.(map[string]any)
	return ref["uid"] != nil || ref["gid"] != nil || ref["mode"] != nil
}

// fileReferenceReport drives the configs/secrets entries above: uid/gid/mode
// on a service-level configs:/secrets: reference are silently ignored
// outside Swarm mode. Each non-nil sub-field on the matched reference
// produces its own finding, identified by the reference's source, so
// multiple references — or multiple flagged sub-fields on the same one —
// stay distinguishable.
func fileReferenceReport(kind string) func(loader.UnsupportedAttribute) []api.UnsupportedAttribute {
	return func(finding loader.UnsupportedAttribute) []api.UnsupportedAttribute {
		ref, _ := finding.Value.(map[string]any)
		// source is schema-optional on the long form (compose-spec's
		// service_config_or_secret has no "required"), so a malformed
		// reference like `configs: [{uid: "1000"}]` is valid enough to
		// reach here without one.
		source, _ := ref["source"].(string)
		if source == "" {
			source = "(anonymous)"
		}
		var findings []api.UnsupportedAttribute
		for _, field := range []string{"uid", "gid", "mode"} {
			if ref[field] == nil {
				continue
			}
			findings = append(findings, api.UnsupportedAttribute{
				Path:   fmt.Sprintf("%s.%s.%s", kind, source, field),
				Reason: field + " is not supported outside Swarm mode and will be ignored",
			})
		}
		return findings
	}
}

// splitServicePath splits a "services.<name>...." finding into its service
// name and the remaining dotted path (matching this package's existing
// attribute-path convention); a project-scoped finding (configs.*/secrets.*)
// has no service and keeps its full path as-is — it already carries its
// resource name as a literal segment.
func splitServicePath(path tree.Path) (service, outputPath string) {
	parts := path.Parts()
	if len(parts) >= 2 && parts[0] == "services" {
		return parts[1], strings.Join(parts[2:], ".")
	}
	return "", path.String()
}

// toAPIUnsupportedAttributes converts compose-go's raw findings to this
// package's api.UnsupportedAttribute. A finding either matches one of
// valueConditionalAttributes (which knows how to render it from Value), or
// it came from the supportedAttributePaths allowlist screening — in which
// case presenceUnsupportedAttributes supplies the reason when this is a
// known case, and genericUnsupportedReason otherwise.
func toAPIUnsupportedAttributes(findings []loader.UnsupportedAttribute) []api.UnsupportedAttribute {
	var result []api.UnsupportedAttribute
	for _, finding := range findings {
		service, outputPath := splitServicePath(finding.Path)

		if vc, findings := matchValueConditional(finding); vc {
			for i := range findings {
				findings[i].Service = service
			}
			result = append(result, findings...)
			continue
		}

		reason := genericUnsupportedReason
		for pattern, r := range presenceUnsupportedAttributes {
			if finding.Path.Matches(pattern) {
				reason = r
				break
			}
		}
		result = append(result, api.UnsupportedAttribute{Service: service, Path: outputPath, Reason: reason})
	}
	slices.SortFunc(result, func(a, b api.UnsupportedAttribute) int {
		if c := cmp.Compare(a.Service, b.Service); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	})
	return result
}

func matchValueConditional(finding loader.UnsupportedAttribute) (bool, []api.UnsupportedAttribute) {
	for _, vc := range valueConditionalAttributes {
		if finding.Path.Matches(vc.Pattern) {
			return true, vc.Report(finding)
		}
	}
	return false, nil
}

// unsupportedAttributesLoadOption registers compose-go's unsupported-attribute
// detection with the loader — both the value-conditional patterns and the
// supported-paths allowlist feed the same walk — invoking report once
// loading completes with every match translated to api.UnsupportedAttribute.
func unsupportedAttributesLoadOption(report func([]api.UnsupportedAttribute)) cli.ProjectOptionsFn {
	patterns := make([]loader.UnsupportedAttributePattern, len(valueConditionalAttributes))
	for i, vc := range valueConditionalAttributes {
		patterns[i] = loader.UnsupportedAttributePattern{Path: vc.Pattern, Detect: vc.Detect}
	}
	wrap := func(findings []loader.UnsupportedAttribute) {
		report(toAPIUnsupportedAttributes(findings))
	}
	return cli.WithLoadOptions(
		loader.WithUnsupportedAttributesCheck(patterns, wrap),
		loader.WithSupportedAttributes(supportedAttributePaths(), nil),
	)
}
