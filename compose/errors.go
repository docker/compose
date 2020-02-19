package compose

import (
	"fmt"
	"strings"
)

func combine(errors []error) error {
	if len(errors) == 0 {
		return nil
	}
	if len(errors) == 1 {
		return errors[0]
	}
	err := combinedError{}
	for _, e := range errors {
		if c, ok := e.(combinedError); ok {
			err.errors = append(err.errors, c.errors...)
		} else {
			err.errors = append(err.errors, e)
		}
	}
	return combinedError{errors}
}

type combinedError struct {
	errors []error
}

func (c combinedError) Error() string {
	points := make([]string, len(c.errors))
	for i, err := range c.errors {
		points[i] = fmt.Sprintf("* %s", err.Error())
	}
	return fmt.Sprintf(
		"%d errors occurred:\n\t%s",
		len(c.errors), strings.Join(points, "\n\t"))
}
