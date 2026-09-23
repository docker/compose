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

package dryrun

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// The dry-run guarantee is a classification of every APIClient method into
// exactly one of three sets, declared here rather than implied by a method's
// position in the file:
//
//   - readOnlyDelegated: reaches the real daemon, must not mutate anything;
//   - faked: intercepted with a plausible in-memory result, so compose
//     operations proceed under --dry-run without touching the engine;
//   - refused: mutating operations compose itself never issues — they
//     return an error instead of silently mutating if future code calls one.
//
// The test parses dryrunclient.go and fails on any method that is missing,
// misclassified, or new and unclassified: adding a method to APIClient
// requires an explicit decision here.
var (
	readOnlyDelegated = set(
		"CheckpointList", "ClientVersion", "Close", "ConfigInspect", "ConfigList", "ContainerDiff",
		"ContainerExport", "ContainerInspect", "ContainerList", "ContainerLogs", "ContainerStatPath", "ContainerStats",
		"ContainerTop", "ContainerWait", "DaemonHost", "DialHijack", "Dialer", "DiskUsage",
		"DistributionInspect", "Events", "ExecInspect", "ImageAttestations", "ImageHistory", "ImageInspect",
		"ImageList", "ImageSave", "ImageSearch", "Info", "NetworkInspect", "NetworkList",
		"NodeInspect", "NodeList", "Ping", "PluginInspect", "PluginList", "RegistryLogin",
		"SecretInspect", "SecretList", "ServerVersion", "ServiceInspect", "ServiceList", "ServiceLogs",
		"SwarmGetUnlockKey", "SwarmInspect", "TaskInspect", "TaskList", "TaskLogs", "VolumeInspect",
		"VolumeList",
	)

	faked = set(
		"ContainerAttach", "ContainerCommit", "ContainerCreate", "ContainerKill", "ContainerPause", "ContainerRemove",
		"ContainerRename", "ContainerRestart", "ContainerStart", "ContainerStop", "ContainerUnpause", "CopyFromContainer",
		"CopyToContainer", "ExecAttach", "ExecCreate", "ExecStart", "ImageBuild", "ImagePull",
		"ImagePush", "ImageRemove", "NetworkConnect", "NetworkCreate", "NetworkDisconnect", "NetworkRemove",
		"VolumeCreate", "VolumeRemove",
	)

	refused = set(
		"BuildCachePrune", "BuildCancel", "CheckpointCreate", "CheckpointRemove", "ConfigCreate", "ConfigRemove",
		"ConfigUpdate", "ContainerPrune", "ContainerResize", "ContainerUpdate", "ExecResize", "ImageImport",
		"ImageLoad", "ImagePrune", "ImageTag", "NetworkPrune", "NodeRemove", "NodeUpdate",
		"PluginCreate", "PluginDisable", "PluginEnable", "PluginInstall", "PluginPush", "PluginRemove",
		"PluginSet", "PluginUpgrade", "SecretCreate", "SecretRemove", "SecretUpdate", "ServiceCreate",
		"ServiceRemove", "ServiceUpdate", "SwarmInit", "SwarmJoin", "SwarmLeave", "SwarmUnlock",
		"SwarmUpdate", "VolumePrune", "VolumeUpdate",
	)
)

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

func TestDryRunClientClassifiesEveryMethod(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "dryrunclient.go", nil, 0)
	assert.NilError(t, err)

	classified := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil {
			continue
		}
		recv, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := recv.X.(*ast.Ident); !ok || ident.Name != "DryRunClient" {
			continue
		}
		name := fn.Name.Name
		if !ast.IsExported(name) {
			continue
		}
		classified[name] = classify(fset, fn, name)
	}

	for name, got := range classified {
		var want string
		switch {
		case readOnlyDelegated[name]:
			want = "delegated"
		case faked[name]:
			want = "faked"
		case refused[name]:
			want = "refused"
		default:
			t.Errorf("method %s is not classified: decide whether it is read-only (delegate), mutating-but-needed (fake it) or mutating-and-unused (refuse it), and add it to the matching list", name)
			continue
		}
		assert.Equal(t, got, want, "method %s", name)
	}
	for _, list := range []map[string]bool{readOnlyDelegated, faked, refused} {
		for name := range list {
			_, found := classified[name]
			assert.Assert(t, found, "classified method %s no longer exists on DryRunClient", name)
		}
	}
}

// classify reads a method body: calling d.apiClient.<Name> means delegated,
// calling errDryRunForbidden means refused, anything else is a fake.
func classify(fset *token.FileSet, fn *ast.FuncDecl, name string) string {
	var src strings.Builder
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if inner, ok := sel.X.(*ast.SelectorExpr); ok {
				if id, ok := inner.X.(*ast.Ident); ok && id.Name == "d" && inner.Sel.Name == "apiClient" {
					src.WriteString("DELEGATE:" + sel.Sel.Name + ";")
				}
			}
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == "errDryRunForbidden" {
			src.WriteString("FORBID;")
		}
		return true
	})
	_ = fset
	if regexp.MustCompile(`DELEGATE:` + name + `;`).MatchString(src.String()) {
		return "delegated"
	}
	if strings.Contains(src.String(), "FORBID;") {
		return "refused"
	}
	return "faked"
}
