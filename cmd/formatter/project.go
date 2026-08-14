/*
   Copyright 2026 Docker Compose CLI authors

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

package formatter

import cliformatter "github.com/docker/cli/cli/command/formatter"

const (
	defaultProjectTableFormat = "table {{.Name}}\t{{.Status}}\t{{.ConfigFiles}}"

	projectNameHeader        = "NAME"
	projectStatusHeader      = "STATUS"
	projectConfigFilesHeader = "CONFIG FILES"
)

// Project is the display model for docker compose ls.
type Project struct {
	Name        string
	Status      string
	ConfigFiles string
}

// NewProjectFormat returns a Docker CLI formatter format for Compose projects.
func NewProjectFormat(source string) cliformatter.Format {
	switch source {
	case cliformatter.TableFormatKey, "":
		return cliformatter.Format(defaultProjectTableFormat)
	case cliformatter.RawFormatKey:
		return `name: {{.Name}}
status: {{.Status}}
config_files: {{.ConfigFiles}}
`
	default:
		return cliformatter.Format(source)
	}
}

// ProjectWrite writes formatted Compose projects using Docker CLI templates.
func ProjectWrite(ctx cliformatter.Context, projects []Project) error {
	render := func(format func(cliformatter.SubContext) error) error {
		for _, project := range projects {
			if err := format(&projectContext{project: project}); err != nil {
				return err
			}
		}
		return nil
	}
	return ctx.Write(newProjectContext(), render)
}

type projectContext struct {
	cliformatter.HeaderContext
	project Project
}

func newProjectContext() *projectContext {
	projectCtx := projectContext{}
	projectCtx.Header = cliformatter.SubHeaderContext{
		"Name":        projectNameHeader,
		"Status":      projectStatusHeader,
		"ConfigFiles": projectConfigFilesHeader,
	}
	return &projectCtx
}

func (c *projectContext) MarshalJSON() ([]byte, error) {
	return cliformatter.MarshalJSON(c)
}

func (c *projectContext) Name() string {
	return c.project.Name
}

func (c *projectContext) Status() string {
	return c.project.Status
}

func (c *projectContext) ConfigFiles() string {
	return c.project.ConfigFiles
}
