package docker

import (
	"encoding/json"
)

type Secret struct {
	ID          string            `json:"ID"`
	Name        string            `json:"Name"`
	Labels      map[string]string `json:"Labels"`
	Description string            `json:"Description"`
}

func (s Secret) ToJSON() (string, error) {
	b, err := json.MarshalIndent(&s, "", "\t")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
