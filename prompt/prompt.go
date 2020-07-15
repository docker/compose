package prompt

import (
	"github.com/AlecAivazis/survey/v2"
)

type UI interface {
	Select(message string, options []string) (int, error)
	Input(message string, defaultValue string) (string, error)
	Confirm(message string, defaultValue bool) (bool, error)
	Password(message string) (string, error)
}

type User struct{}

func (u User) Select(message string, options []string) (int, error) {
	qs := &survey.Select{
		Message: message,
		Options: options,
	}
	var selected int
	err := survey.AskOne(qs, &selected, nil)
	return selected, err
}
func (u User) Input(message string, defaultValue string) (string, error) {
	qs := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	var s string
	err := survey.AskOne(qs, &s, nil)
	return s, err
}

func (u User) Confirm(message string, defaultValue bool) (bool, error) {
	qs := &survey.Confirm{
		Message: message,
		Default: defaultValue,
	}
	var b bool
	err := survey.AskOne(qs, &b, nil)
	return b, err
}

func (u User) Password(message string) (string, error) {
	qs := &survey.Password{
		Message: message,
	}
	var p string
	err := survey.AskOne(qs, &p, nil)
	return p, err

}
