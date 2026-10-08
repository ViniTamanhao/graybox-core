// Package config reads optional project configuration without resolving secrets.
package config

import (
	"bytes"
	"errors"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

type Redact struct {
	Headers []string `yaml:"headers"`
	JSON    []string `yaml:"json"`
	Query   []string `yaml:"query"`
	Form    []string `yaml:"form"`
}

type Replay struct {
	Headers map[string]string `yaml:"headers"`
	JSON    map[string]string `yaml:"json"`
	Query   map[string]string `yaml:"query"`
	Form    map[string]string `yaml:"form"`
}

type Config struct {
	Redact Redact `yaml:"redact"`
	Replay Replay `yaml:"replay"`
}

// Load discovers only in the current directory. An explicit path must exist.
func Load(path string) (Config, error) {
	explicit := path != ""
	if !explicit {
		path = "graybox.yaml"
	}
	data, err := os.ReadFile(path)
	if !explicit && errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, errors.New("cannot read graybox configuration")
	}
	return Parse(data)
}

func Parse(data []byte) (Config, error) {
	var result Config
	invalid := errors.New("invalid graybox configuration: expected one YAML document with only redact/replay headers, json, query, and form keys")
	// Check scalar types, aliases and duplicates before decoding; YAML string
	// coercion would otherwise accept booleans/numbers for credentials or paths.
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return result, invalid
	}
	if !validNode(&node) {
		return result, invalid
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, invalid
	}
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(&result); err != nil {
		return Config{}, invalid
	}
	return result, nil
}

func validNode(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.DocumentNode:
		return len(node.Content) == 1 && node.Content[0].Kind == yaml.MappingNode && validNode(node.Content[0])
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return false
		}
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return false
			}
			seen[key.Value] = true
			if !validNode(node.Content[i+1]) {
				return false
			}
		}
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return false
		}
		for _, child := range node.Content {
			if !validNode(child) {
				return false
			}
		}
	case yaml.ScalarNode:
		return node.Tag == "!!str"
	default:
		return false
	}
	return true
}
