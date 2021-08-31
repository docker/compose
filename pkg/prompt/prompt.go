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

package prompt

import (
	"github.com/AlecAivazis/survey/v2"
)

//go:generate mockgen -destination=./prompt_mock.go -self_package "github.com/docker/compose/v2/pkg/prompt" -package=prompt . UI

// UI - prompt user input
type UI interface {
	Select(message string, options []string) (int, error)
	Input(message string, defaultValue string) (string, error)
	Confirm(message string, defaultValue bool) (bool, error)
	Password(message string) (string, error)
}

// User - aggregates prompt methods
type User struct{}

// Select - displays a list
func (u User) Select(message string, options []string) (int, error) {
	qs := &survey.Select{
		Message: message,
		Options: options,
	}
	var selected int
	err := survey.AskOne(qs, &selected, nil)
	return selected, err
}

// Input text with default value
func (u User) Input(message string, defaultValue string) (string, error) {
	qs := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	var s string
	err := survey.AskOne(qs, &s, nil)
	return s, err
}

// Confirm asks for yes or no input
func (u User) Confirm(message string, defaultValue bool) (bool, error) {
	qs := &survey.Confirm{
		Message: message,
		Default: defaultValue,
	}
	var b bool
	err := survey.AskOne(qs, &b, nil)
	return b, err
}

// Password implements a text input with masked characters.
func (u User) Password(message string) (string, error) {
	qs := &survey.Password{
		Message: message,
	}
	var p string
	err := survey.AskOne(qs, &p, nil)
	return p, err

}
