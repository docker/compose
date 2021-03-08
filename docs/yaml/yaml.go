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

package yaml

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	yaml "gopkg.in/yaml.v3"
)

type cmdOption struct {
	Option          string
	Shorthand       string `yaml:",omitempty"`
	ValueType       string `yaml:"value_type,omitempty"`
	DefaultValue    string `yaml:"default_value,omitempty"`
	Description     string `yaml:",omitempty"`
	Deprecated      bool
	MinAPIVersion   string `yaml:"min_api_version,omitempty"`
	Experimental    bool
	ExperimentalCLI bool
	Kubernetes      bool
	Swarm           bool
	OSType          string `yaml:"os_type,omitempty"`
}

type cmdDoc struct {
	Name             string      `yaml:"command"`
	SeeAlso          []string    `yaml:"parent,omitempty"`
	Version          string      `yaml:"engine_version,omitempty"`
	Aliases          string      `yaml:",omitempty"`
	Short            string      `yaml:",omitempty"`
	Long             string      `yaml:",omitempty"`
	Usage            string      `yaml:",omitempty"`
	Pname            string      `yaml:",omitempty"`
	Plink            string      `yaml:",omitempty"`
	Cname            []string    `yaml:",omitempty"`
	Clink            []string    `yaml:",omitempty"`
	Options          []cmdOption `yaml:",omitempty"`
	InheritedOptions []cmdOption `yaml:"inherited_options,omitempty"`
	Example          string      `yaml:"examples,omitempty"`
	Deprecated       bool
	MinAPIVersion    string `yaml:"min_api_version,omitempty"`
	Experimental     bool
	ExperimentalCLI  bool
	Kubernetes       bool
	Swarm            bool
	OSType           string `yaml:"os_type,omitempty"`
}

// GenYamlTree creates yaml structured ref files
func GenYamlTree(cmd *cobra.Command, dir string) error {
	emptyStr := func(s string) string { return "" }
	return GenYamlTreeCustom(cmd, dir, emptyStr)
}

// GenYamlTreeCustom creates yaml structured ref files
func GenYamlTreeCustom(cmd *cobra.Command, dir string, filePrepender func(string) string) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err := GenYamlTreeCustom(c, dir, filePrepender); err != nil {
			return err
		}
	}
	if !cmd.HasParent() {
		return nil
	}

	basename := strings.Replace(cmd.CommandPath(), " ", "_", -1) + ".yaml"
	filename := filepath.Join(dir, basename)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close() //nolint: errcheck

	if _, err := io.WriteString(f, filePrepender(filename)); err != nil {
		return err
	}
	return GenYamlCustom(cmd, f)
}

// GenYamlCustom creates custom yaml output
// nolint: gocyclo
func GenYamlCustom(cmd *cobra.Command, w io.Writer) error {
	cliDoc := cmdDoc{}
	cliDoc.Name = cmd.CommandPath()

	cliDoc.Aliases = strings.Join(cmd.Aliases, ", ")
	cliDoc.Short = cmd.Short
	cliDoc.Long = cmd.Long
	if len(cliDoc.Long) == 0 {
		cliDoc.Long = cliDoc.Short
	}

	if cmd.Runnable() {
		cliDoc.Usage = cmd.UseLine()
	}

	if len(cmd.Example) > 0 {
		cliDoc.Example = cmd.Example
	}
	if len(cmd.Deprecated) > 0 {
		cliDoc.Deprecated = true
	}
	// Check recursively so that, e.g., `docker stack ls` returns the same output as `docker stack`
	for curr := cmd; curr != nil; curr = curr.Parent() {
		if v, ok := curr.Annotations["version"]; ok && cliDoc.MinAPIVersion == "" {
			cliDoc.MinAPIVersion = v
		}
		if _, ok := curr.Annotations["experimental"]; ok && !cliDoc.Experimental {
			cliDoc.Experimental = true
		}
		if _, ok := curr.Annotations["experimentalCLI"]; ok && !cliDoc.ExperimentalCLI {
			cliDoc.ExperimentalCLI = true
		}
		if _, ok := curr.Annotations["kubernetes"]; ok && !cliDoc.Kubernetes {
			cliDoc.Kubernetes = true
		}
		if _, ok := curr.Annotations["swarm"]; ok && !cliDoc.Swarm {
			cliDoc.Swarm = true
		}
		if os, ok := curr.Annotations["ostype"]; ok && cliDoc.OSType == "" {
			cliDoc.OSType = os
		}
	}

	flags := cmd.NonInheritedFlags()
	if flags.HasFlags() {
		cliDoc.Options = genFlagResult(flags)
	}
	flags = cmd.InheritedFlags()
	if flags.HasFlags() {
		cliDoc.InheritedOptions = genFlagResult(flags)
	}

	if hasSeeAlso(cmd) {
		if cmd.HasParent() {
			parent := cmd.Parent()
			cliDoc.Pname = parent.CommandPath()
			link := cliDoc.Pname + ".yaml"
			cliDoc.Plink = strings.Replace(link, " ", "_", -1)
			cmd.VisitParents(func(c *cobra.Command) {
				if c.DisableAutoGenTag {
					cmd.DisableAutoGenTag = c.DisableAutoGenTag
				}
			})
		}

		children := cmd.Commands()
		sort.Sort(byName(children))

		for _, child := range children {
			if !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
				continue
			}
			currentChild := cliDoc.Name + " " + child.Name()
			cliDoc.Cname = append(cliDoc.Cname, cliDoc.Name+" "+child.Name())
			link := currentChild + ".yaml"
			cliDoc.Clink = append(cliDoc.Clink, strings.Replace(link, " ", "_", -1))
		}
	}

	final, err := yaml.Marshal(&cliDoc)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if _, err := fmt.Fprintln(w, string(final)); err != nil {
		return err
	}
	return nil
}

func genFlagResult(flags *pflag.FlagSet) []cmdOption {
	var (
		result []cmdOption
		opt    cmdOption
	)

	flags.VisitAll(func(flag *pflag.Flag) {
		opt = cmdOption{
			Option:       flag.Name,
			ValueType:    flag.Value.Type(),
			DefaultValue: forceMultiLine(flag.DefValue),
			Description:  forceMultiLine(flag.Usage),
			Deprecated:   len(flag.Deprecated) > 0,
		}

		// Todo, when we mark a shorthand is deprecated, but specify an empty message.
		// The flag.ShorthandDeprecated is empty as the shorthand is deprecated.
		// Using len(flag.ShorthandDeprecated) > 0 can't handle this, others are ok.
		if !(len(flag.ShorthandDeprecated) > 0) && len(flag.Shorthand) > 0 {
			opt.Shorthand = flag.Shorthand
		}
		if _, ok := flag.Annotations["experimental"]; ok {
			opt.Experimental = true
		}
		if v, ok := flag.Annotations["version"]; ok {
			opt.MinAPIVersion = v[0]
		}
		if _, ok := flag.Annotations["experimentalCLI"]; ok {
			opt.ExperimentalCLI = true
		}
		if _, ok := flag.Annotations["kubernetes"]; ok {
			opt.Kubernetes = true
		}
		if _, ok := flag.Annotations["swarm"]; ok {
			opt.Swarm = true
		}

		// Note that the annotation can have multiple ostypes set, however, multiple
		// values are currently not used (and unlikely will).
		//
		// To simplify usage of the os_type property in the YAML, and for consistency
		// with the same property for commands, we're only using the first ostype that's set.
		if ostypes, ok := flag.Annotations["ostype"]; ok && len(opt.OSType) == 0 && len(ostypes) > 0 {
			opt.OSType = ostypes[0]
		}

		result = append(result, opt)
	})

	return result
}

// Temporary workaround for yaml lib generating incorrect yaml with long strings
// that do not contain \n.
func forceMultiLine(s string) string {
	if len(s) > 60 && !strings.Contains(s, "\n") {
		s = s + "\n"
	}
	return s
}

// Small duplication for cobra utils
func hasSeeAlso(cmd *cobra.Command) bool {
	if cmd.HasParent() {
		return true
	}
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		return true
	}
	return false
}

// ParseMDContent parse markdown file looking for Description and Examples sections
func ParseMDContent(mdString string) (description string, examples string) {
	parsedContent := strings.Split(mdString, "\n## ")
	for _, s := range parsedContent {
		if strings.Index(s, "Description") == 0 {
			description = strings.TrimSpace(strings.TrimPrefix(s, "Description"))
		}
		if strings.Index(s, "Examples") == 0 {
			examples = strings.TrimSpace(strings.TrimPrefix(s, "Examples"))
		}
	}
	return description, examples
}

type byName []*cobra.Command

func (s byName) Len() int           { return len(s) }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byName) Less(i, j int) bool { return s[i].Name() < s[j].Name() }
