package prompt

import (
	"github.com/AlecAivazis/survey/v2"
)

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

// Password  implemetns a text input with masked characters
func (u User) Password(message string) (string, error) {
	qs := &survey.Password{
		Message: message,
	}
	var p string
	err := survey.AskOne(qs, &p, nil)
	return p, err

}
