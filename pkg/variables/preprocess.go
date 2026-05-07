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

package variables

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/template"
	"github.com/sirupsen/logrus"
	"go.yaml.in/yaml/v4"
)

// Result is what a successful Render returns.
type Result struct {
	// ConfigPaths are the rendered file paths (in tempdir) to feed
	// to compose-go in place of the user's originals.
	ConfigPaths []string
	// Cleanup removes the tempdir. Always non-nil; safe to defer.
	Cleanup func()
	// Debug lists every variable declaration encountered, with
	// provenance, in the order it was discovered.
	Debug []DebugEntry
	// Resolved is the per-name final value used during interpolation
	// (root scope). Shell-overridden vars show the shell value.
	Resolved map[string]string
}

// DebugEntry is a debug-mode dump row.
type DebugEntry struct {
	Name     string
	Value    string // raw declared value (before substitution)
	Resolved string // post-substitution / shell-overridden value
	Source   Source
	From     string
	Active   bool // true if this entry is the winning source for its name
}

// Render preprocesses Compose YAML files: extracts `variables:` blocks
// and include-local variable declarations, performs interpolation with
// the resolved scope, strips the extension keys, and writes cleaned
// YAML files to a tempdir. The returned ConfigPaths can be fed to
// compose-go (which must be told to skip its own interpolation).
//
// shell may be nil; when nil, os.LookupEnv is used.
func Render(ctx context.Context, configPaths []string, cliVars []string, cliVarFiles []string, shell func(string) (string, bool)) (*Result, error) {
	return renderInternal(ctx, configPaths, cliVars, cliVarFiles, shell, true)
}

// Strip behaves like Render but does NOT substitute string leaves; it
// only strips the extension keys (`variables:`, `variables_file:`,
// include-local `variables`/`variables_file`) so compose-go can load
// the file without rejecting the unknown top-level keys. Use this for
// callers that need the raw model (e.g. `--no-interpolate`,
// `ExtractVariables`).
func Strip(ctx context.Context, configPaths []string, cliVars []string, cliVarFiles []string, shell func(string) (string, bool)) (*Result, error) {
	return renderInternal(ctx, configPaths, cliVars, cliVarFiles, shell, false)
}

func renderInternal(ctx context.Context, configPaths []string, cliVars []string, cliVarFiles []string, shell func(string) (string, bool), interpolate bool) (*Result, error) {
	if shell == nil {
		shell = os.LookupEnv
	}

	tempdir, err := os.MkdirTemp("", "compose-variables-")
	if err != nil {
		return nil, fmt.Errorf("create tempdir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempdir)
	}

	rt := &renderer{
		ctx:         ctx,
		tempdir:     tempdir,
		visited:     map[string]string{},
		shell:       shell,
		interpolate: interpolate,
	}

	rootScope, err := buildCLIScope(cliVars, cliVarFiles, shell)
	if err != nil {
		cleanup()
		return nil, err
	}

	roots, err := readRootFiles(configPaths)
	if err != nil {
		cleanup()
		return nil, err
	}

	if err := mergeRootVariables(rootScope, roots); err != nil {
		cleanup()
		return nil, err
	}

	rendered := make([]string, 0, len(roots))
	for _, r := range roots {
		out, err := rt.processFile(r.absPath, r.node, rootScope)
		if err != nil {
			cleanup()
			return nil, err
		}
		rendered = append(rendered, out)
	}

	resolved, err := rootScope.Resolve()
	if err != nil {
		cleanup()
		return nil, err
	}
	debug := buildDebug(rootScope, resolved, shell)

	return &Result{
		ConfigPaths: rendered,
		Cleanup:     cleanup,
		Debug:       debug,
		Resolved:    resolved,
	}, nil
}

type rootFile struct {
	absPath  string
	node     *yaml.Node
	topVars  *declaredBlock
	varFiles []string // parsed list from a top-level `variables_file:` key
}

func buildCLIScope(cliVars, cliVarFiles []string, shell func(string) (string, bool)) (*Scope, error) {
	cliEntries, err := ParseCLIVars(cliVars)
	if err != nil {
		return nil, err
	}
	var cliFileEntries []Entry
	for _, p := range cliVarFiles {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		entries, err := LoadVarsFile(abs, SourceCLIVarFile)
		if err != nil {
			return nil, err
		}
		cliFileEntries = append(cliFileEntries, entries...)
	}
	scope := NewScope(shell)
	scope.AddAll(cliEntries)
	scope.AddAll(cliFileEntries)
	return scope, nil
}

func readRootFiles(configPaths []string) ([]rootFile, error) {
	roots := make([]rootFile, 0, len(configPaths))
	for _, p := range configPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		node, err := readYAMLNode(abs)
		if err != nil {
			return nil, err
		}
		var block *declaredBlock
		if varsNode := stripTopLevelKey(node, "variables"); varsNode != nil {
			block, err = parseDeclaredOrdered(varsNode)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", abs, err)
			}
		}
		var varFiles []string
		if vfNode := stripTopLevelKey(node, "variables_file"); vfNode != nil {
			varFiles, err = nodeToStringList(vfNode)
			if err != nil {
				return nil, fmt.Errorf("%s: variables_file: %w", abs, err)
			}
		}
		roots = append(roots, rootFile{absPath: abs, node: node, topVars: block, varFiles: varFiles})
	}
	return roots, nil
}

func mergeRootVariables(scope *Scope, roots []rootFile) error {
	// Inline first (higher precedence than variables_file entries).
	for _, r := range roots {
		if r.topVars == nil {
			continue
		}
		for _, e := range r.topVars.Inline {
			val, err := Coerce(e.Name, e.Value)
			if err != nil {
				return fmt.Errorf("%s: %w", r.absPath, err)
			}
			scope.Add(Entry{Name: e.Name, Value: val, Source: SourceRootInline, From: r.absPath})
		}
	}
	// variables_file: later files win, so add later FIRST.
	for _, r := range roots {
		for i := len(r.varFiles) - 1; i >= 0; i-- {
			fp := r.varFiles[i]
			if !filepath.IsAbs(fp) {
				fp = filepath.Join(filepath.Dir(r.absPath), fp)
			}
			entries, err := LoadVarsFile(fp, SourceRootFile)
			if err != nil {
				return err
			}
			scope.AddAll(entries)
		}
	}
	return nil
}

// renderer carries shared state for a Render invocation.
type renderer struct {
	ctx     context.Context
	tempdir string
	// visited maps abs source path → rendered abs path. Prevents
	// re-rendering and detects cycles.
	visited map[string]string
	shell   func(string) (string, bool)
	// interpolate controls whether string leaves get substituted.
	// When false, the renderer only strips extension keys (used by
	// `compose config --no-interpolate` / `--variables`).
	interpolate bool
}

// processFile renders one Compose file (root or included) into the
// tempdir using parentScope. Returns the rendered absolute path.
func (rt *renderer) processFile(absPath string, node *yaml.Node, parentScope *Scope) (string, error) {
	if existing, ok := rt.visited[absPath]; ok {
		return existing, nil
	}
	rt.visited[absPath] = ""

	scope := parentScope.Inherit()
	if err := mergeOwnVariables(scope, absPath, node); err != nil {
		return "", err
	}

	if includeNode := findTopLevelKey(node, "include"); includeNode != nil {
		if err := rt.processIncludes(absPath, includeNode, scope); err != nil {
			return "", err
		}
	}

	// Decode (now-stripped) node into a generic model so we can
	// substitute string leaves and re-marshal.
	var model any
	if err := node.Decode(&model); err != nil {
		return "", fmt.Errorf("%s: decode: %w", absPath, err)
	}

	if rt.interpolate {
		resolved, err := scope.Resolve()
		if err != nil {
			return "", fmt.Errorf("%s: %w", absPath, err)
		}
		mapping := scope.Mapping(resolved)
		model = substituteAny(model, mapping)
	}

	// Write rendered file.
	out := mirrorPath(rt.tempdir, absPath)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(model)
	if err != nil {
		return "", fmt.Errorf("%s: marshal: %w", absPath, err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return "", err
	}
	rt.visited[absPath] = out
	return out, nil
}

// processIncludes walks the include: sequence, mutating each entry
// to (a) strip variables/variables_file keys, (b) rewrite path to
// rendered tempdir paths, (c) preserve project_directory pointing at
// the original directory so relative paths inside included files
// still resolve.
func (rt *renderer) processIncludes(parentAbsPath string, includeNode *yaml.Node, parentScope *Scope) error {
	if includeNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s: `include:` must be a sequence", parentAbsPath)
	}
	for _, entry := range includeNode.Content {
		if err := rt.processIncludeEntry(parentAbsPath, entry, parentScope); err != nil {
			return err
		}
	}
	return nil
}

func (rt *renderer) processIncludeEntry(parentAbsPath string, entry *yaml.Node, parentScope *Scope) error {
	normalizeIncludeEntry(entry)
	if entry.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: include entry must be a string or mapping", parentAbsPath)
	}

	localVarsNode := stripMappingKey(entry, "variables")
	localVarsFileNode := stripMappingKey(entry, "variables_file")

	pathNode := mappingValue(entry, "path")
	if pathNode == nil {
		return fmt.Errorf("%s: include entry missing `path:`", parentAbsPath)
	}
	paths, err := nodeToStringList(pathNode)
	if err != nil {
		return fmt.Errorf("%s: include.path: %w", parentAbsPath, err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("%s: include.path is empty", parentAbsPath)
	}

	parentDir := filepath.Dir(parentAbsPath)
	absPaths := make([]string, len(paths))
	for i, p := range paths {
		if filepath.IsAbs(p) {
			absPaths[i] = p
		} else {
			absPaths[i] = filepath.Join(parentDir, p)
		}
	}

	origProjectDir := resolveIncludeProjectDir(entry, parentDir, absPaths[0])

	includeScope, err := buildIncludeScope(parentScope, parentAbsPath, localVarsNode, localVarsFileNode, origProjectDir)
	if err != nil {
		return err
	}

	renderedAbs, err := rt.renderIncludedFiles(absPaths, includeScope)
	if err != nil {
		return err
	}

	rewriteIncludeEntry(entry, renderedAbs, origProjectDir)
	return nil
}

// normalizeIncludeEntry converts string-form (`include: ./x.yaml`) into
// mapping form so we can attach project_directory consistently.
func normalizeIncludeEntry(entry *yaml.Node) {
	if entry.Kind != yaml.ScalarNode {
		return
	}
	pathStr := entry.Value
	*entry = yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "path"},
			{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: pathStr},
			}},
		},
	}
}

// resolveIncludeProjectDir returns the original-FS project_directory
// for an include entry. Defaults to dir of first path.
func resolveIncludeProjectDir(entry *yaml.Node, parentDir, firstAbsPath string) string {
	if pdNode := mappingValue(entry, "project_directory"); pdNode != nil && pdNode.Value != "" {
		if filepath.IsAbs(pdNode.Value) {
			return pdNode.Value
		}
		return filepath.Join(parentDir, pdNode.Value)
	}
	return filepath.Dir(firstAbsPath)
}

// buildIncludeScope builds the scope used for an include's contents.
func buildIncludeScope(parentScope *Scope, parentAbsPath string, localVarsNode, localVarsFileNode *yaml.Node, origProjectDir string) (*Scope, error) {
	includeScope := parentScope.Inherit()
	if localVarsNode != nil {
		block, err := parseDeclaredOrdered(localVarsNode)
		if err != nil {
			return nil, fmt.Errorf("%s: include.variables: %w", parentAbsPath, err)
		}
		for _, e := range block.Inline {
			val, err := Coerce(e.Name, e.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: include.variables: %w", parentAbsPath, err)
			}
			includeScope.Add(Entry{Name: e.Name, Value: val, Source: SourceIncludeInline, From: parentAbsPath})
		}
	}
	if localVarsFileNode != nil {
		files, err := nodeToStringList(localVarsFileNode)
		if err != nil {
			return nil, fmt.Errorf("%s: include.variables_file: %w", parentAbsPath, err)
		}
		// Resolve against project_directory (or include entry dir if unset).
		for i := len(files) - 1; i >= 0; i-- {
			fp := files[i]
			if !filepath.IsAbs(fp) {
				fp = filepath.Join(origProjectDir, fp)
			}
			entries, err := LoadVarsFile(fp, SourceIncludeFile)
			if err != nil {
				return nil, err
			}
			includeScope.AddAll(entries)
		}
	}
	return includeScope, nil
}

func (rt *renderer) renderIncludedFiles(absPaths []string, includeScope *Scope) ([]string, error) {
	out := make([]string, len(absPaths))
	for i, abs := range absPaths {
		incNode, err := readYAMLNode(abs)
		if err != nil {
			return nil, err
		}
		if reserved, ok := rt.visited[abs]; ok && reserved == "" {
			return nil, fmt.Errorf("include cycle detected at %s", abs)
		}
		rendered, err := rt.processFile(abs, incNode, includeScope)
		if err != nil {
			return nil, err
		}
		out[i] = rendered
	}
	return out, nil
}

func rewriteIncludeEntry(entry *yaml.Node, renderedAbs []string, origProjectDir string) {
	newPathSeq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, p := range renderedAbs {
		newPathSeq.Content = append(newPathSeq.Content, &yaml.Node{
			Kind: yaml.ScalarNode, Tag: "!!str", Value: p,
		})
	}
	setMappingValue(entry, "path", newPathSeq)
	setMappingValue(entry, "project_directory", &yaml.Node{
		Kind: yaml.ScalarNode, Tag: "!!str", Value: origProjectDir,
	})
}

// mergeOwnVariables strips and merges a file's own top-level
// `variables:` and `variables_file:` blocks into scope.
func mergeOwnVariables(scope *Scope, absPath string, node *yaml.Node) error {
	if varsNode := stripTopLevelKey(node, "variables"); varsNode != nil {
		block, err := parseDeclaredOrdered(varsNode)
		if err != nil {
			return fmt.Errorf("%s: %w", absPath, err)
		}
		for _, e := range block.Inline {
			val, err := Coerce(e.Name, e.Value)
			if err != nil {
				return fmt.Errorf("%s: %w", absPath, err)
			}
			scope.Add(Entry{Name: e.Name, Value: val, Source: SourceIncludedTopLevel, From: absPath})
		}
	}
	if vfNode := stripTopLevelKey(node, "variables_file"); vfNode != nil {
		files, err := nodeToStringList(vfNode)
		if err != nil {
			return fmt.Errorf("%s: variables_file: %w", absPath, err)
		}
		for i := len(files) - 1; i >= 0; i-- {
			fp := files[i]
			if !filepath.IsAbs(fp) {
				fp = filepath.Join(filepath.Dir(absPath), fp)
			}
			entries, err := LoadVarsFile(fp, SourceIncludedTopLevel)
			if err != nil {
				return err
			}
			scope.AddAll(entries)
		}
	}
	return nil
}

// readYAMLNode reads a YAML file and returns its document node.
func readYAMLNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind == 0 {
		// empty file
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("parse %s: not a YAML document", path)
	}
	return &doc, nil
}

// stripTopLevelKey removes (key, value) from the document's root
// mapping and returns the value node (or nil).
func stripTopLevelKey(doc *yaml.Node, key string) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Kind == yaml.ScalarNode && root.Content[i].Value == key {
			val := root.Content[i+1]
			root.Content = append(root.Content[:i], root.Content[i+2:]...)
			return val
		}
	}
	return nil
}

// findTopLevelKey returns the value node for a top-level key, or nil.
func findTopLevelKey(doc *yaml.Node, key string) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Kind == yaml.ScalarNode && root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

// stripMappingKey removes (key, value) from a mapping node and
// returns the removed value (or nil).
func stripMappingKey(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Kind == yaml.ScalarNode && m.Content[i].Value == key {
			val := m.Content[i+1]
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return val
		}
	}
	return nil
}

// mappingValue returns the value node for a key (or nil).
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Kind == yaml.ScalarNode && m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMappingValue replaces or appends key=val on a mapping node.
func setMappingValue(m *yaml.Node, key string, val *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Kind == yaml.ScalarNode && m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		val,
	)
}

// substituteAny recursively walks a generic decoded YAML model and
// runs template substitution on every string leaf using mapping.
func substituteAny(v any, mapping template.Mapping) any {
	switch x := v.(type) {
	case map[string]any:
		for k, item := range x {
			x[k] = substituteAny(item, mapping)
		}
		return x
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			ks, ok := k.(string)
			if !ok {
				ks = fmt.Sprint(k)
			}
			out[ks] = substituteAny(item, mapping)
		}
		return out
	case []any:
		for i, item := range x {
			x[i] = substituteAny(item, mapping)
		}
		return x
	case string:
		out, err := template.SubstituteWithOptions(x, missWarnMapping(mapping), template.WithoutLogging)
		if err != nil {
			// Required-error or syntax error: keep original; let
			// compose-go surface it later if it cares. Still log.
			logrus.Warnf("interpolation error on %q: %v", x, err)
			return x
		}
		return out
	}
	return v
}

// missWarnMapping wraps a mapping so unresolved names log a warning
// before returning empty/false. Mirrors compose-go's interpolation
// default behavior.
func missWarnMapping(inner template.Mapping) template.Mapping {
	return func(name string) (string, bool) {
		v, ok := inner(name)
		if !ok {
			logrus.Warnf("variable %q is not declared and not set in shell environment", name)
		}
		return v, ok
	}
}

// mirrorPath returns the tempdir path that mirrors absPath.
func mirrorPath(tempdir, absPath string) string {
	clean := absPath
	if filepath.IsAbs(clean) {
		// Strip any drive letter / leading separator portably.
		vol := filepath.VolumeName(clean)
		clean = strings.TrimPrefix(clean, vol)
		clean = strings.TrimPrefix(clean, string(filepath.Separator))
	}
	return filepath.Join(tempdir, clean)
}

// buildDebug produces a per-name debug entry list. Active=true marks
// the winning entry. Shell-overridden vars get an extra synthetic
// entry tagged SourceShell.
func buildDebug(scope *Scope, resolved map[string]string, shell func(string) (string, bool)) []DebugEntry {
	winners := map[string]Entry{}
	for _, e := range scope.Winners() {
		winners[e.Name] = e
	}
	out := make([]DebugEntry, 0, len(scope.All()))
	for _, e := range scope.All() {
		w := winners[e.Name]
		active := w.Source == e.Source && w.From == e.From && w.Value == e.Value
		var resolvedVal string
		if v, ok := shell(e.Name); ok {
			resolvedVal = v
		} else if v, ok := resolved[e.Name]; ok {
			resolvedVal = v
		}
		out = append(out, DebugEntry{
			Name:     e.Name,
			Value:    e.Value,
			Resolved: resolvedVal,
			Source:   e.Source,
			From:     e.From,
			Active:   active && !shellHas(shell, e.Name),
		})
	}
	// If shell overrides a declared name, add a SourceShell entry on
	// top so debug output reflects who actually won.
	for name := range winners {
		if v, ok := shell(name); ok {
			out = append(out, DebugEntry{
				Name:     name,
				Value:    v,
				Resolved: v,
				Source:   SourceShell,
				From:     "shell environment",
				Active:   true,
			})
		}
	}
	return out
}

func shellHas(shell func(string) (string, bool), name string) bool {
	_, ok := shell(name)
	return ok
}
