package tachikoma

import (
	"reflect"
	"testing"
)

func TestParseGoMod(t *testing.T) {
	mockGoMod := `
module github.com/Hoosk/motoko

go 1.24

require (
	github.com/charmbracelet/bubbletea v0.25.0
	github.com/smacker/go-tree-sitter v1.2.3
)

require github.com/stretchr/testify v1.8.4 // indirect
`

	moduleName, deps := parseGoMod(mockGoMod)

	if moduleName != "github.com/Hoosk/motoko" {
		t.Errorf("Expected module name 'github.com/Hoosk/motoko', got %s", moduleName)
	}

	expectedDeps := []string{
		"github.com/charmbracelet/bubbletea",
		"github.com/smacker/go-tree-sitter",
		"github.com/stretchr/testify",
	}

	if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("Expected deps %v, got %v", expectedDeps, deps)
	}
}

func TestParsePackageJSON(t *testing.T) {
	mockJSON := `{
		"name": "my-cool-app",
		"version": "1.0.0",
		"dependencies": {
			"express": "^4.17.1",
			"lodash": "^4.17.21"
		},
		"devDependencies": {
			"typescript": "^4.5.2"
		}
	}`

	name, deps := parsePackageJSON(mockJSON)
	if name != "my-cool-app" {
		t.Errorf("Expected package name 'my-cool-app', got %s", name)
	}

	expectedDeps := []string{
		"express",
		"lodash",
		"typescript",
	}

	if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("Expected deps %v, got %v", expectedDeps, deps)
	}

	// Test invalid JSON
	nameErr, depsErr := parsePackageJSON("{invalid json")
	if nameErr != "" || depsErr != nil {
		t.Errorf("Expected empty result for invalid JSON, got name=%q, deps=%v", nameErr, depsErr)
	}
}

func TestParseCargoTOML(t *testing.T) {
	mockCargo := `
[package]
name = "motoko-rs"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = { version = "1.0", features = ["derive"] }
tokio = "1.0"

[dev-dependencies]
tempfile = "3.2"

[build-dependencies]
cc = "1.0"
`

	name, deps := parseCargoTOML(mockCargo)
	if name != "motoko-rs" {
		t.Errorf("Expected name 'motoko-rs', got %s", name)
	}

	expectedDeps := []string{
		"cc",
		"serde",
		"tempfile",
		"tokio",
	}

	if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("Expected deps %v, got %v", expectedDeps, deps)
	}
}

func TestParseRequirementsTXT(t *testing.T) {
	mockReqs := `
# Simple dependency
requests==2.26.0
numpy>=1.20
# Dependency with extras
pandas[excel]; python_version > '3.6'
# Dependency in edit mode / file reference (should be filtered or parsed cleanly)
-r dev-requirements.txt

  flask
`

	deps := parseRequirementsTXT(mockReqs)
	expectedDeps := []string{
		"flask",
		"numpy",
		"pandas",
		"requests",
	}

	if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("Expected deps %v, got %v", expectedDeps, deps)
	}
}

func TestParseGemfile(t *testing.T) {
	mockGemfile := `
source 'https://rubygems.org'

# Ruby version
ruby '3.0.0'

gem 'rails', '~> 7.0'
gem "pg", ">= 0.18"
gem 'puma' # Web server

# Development gems
group :development do
  gem 'byebug'
end
`

	deps := parseGemfile(mockGemfile)
	expectedDeps := []string{
		"byebug",
		"pg",
		"puma",
		"rails",
	}

	if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("Expected deps %v, got %v", expectedDeps, deps)
	}
}
