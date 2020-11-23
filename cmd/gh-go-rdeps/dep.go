package main

import (
	"io"

	"github.com/pelletier/go-toml"
)

type GopkgProject struct {
	Name     string `toml:"name"`
	Branch   string `toml:"branch,omitempty"`
	Revision string `toml:"revision,omitempty"`
	Version  string `toml:"version,omitempty"`
	Source   string `toml:"source,omitempty"`
}

type Gopkg struct {
	Constraints []GopkgProject `toml:"constraint,omitempty"`
	Overrides   []GopkgProject `toml:"override,omitempty"`
	Ignored     []string       `toml:"ignored,omitempty"`
	Required    []string       `toml:"required,omitempty"`
}

func parseGopkg(r io.Reader) (*Gopkg, error) {
	gopkg := &Gopkg{}
	err := toml.NewDecoder(r).Decode(gopkg)
	if err != nil {
		return nil, err
	}

	return gopkg, nil
}
