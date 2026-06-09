package tachikoma

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type ProjectDependencies struct {
	Ecosystems map[string][]string
}

type DependencyTachikoma struct {
	interval time.Duration
}

func NewDependencyTachikoma() *DependencyTachikoma {
	return &DependencyTachikoma{
		interval: 15 * time.Second,
	}
}

func (d *DependencyTachikoma) Name() string {
	return "DependencyTachikoma"
}

func (d *DependencyTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	// Watch for changes in workspace
	events, _ := WatchHelper(ctx, []string{"."}, 1*time.Second)

	refresh := func() {
		ecosystems := make(map[string][]string)

		// 1. Go (go.mod)
		if content, err := os.ReadFile("go.mod"); err == nil {
			_, deps := parseGoMod(string(content))
			if len(deps) > 0 {
				ecosystems["Go"] = deps
			}
		}

		// 2. Node/JS/TS (package.json)
		if content, err := os.ReadFile("package.json"); err == nil {
			_, deps := parsePackageJSON(string(content))
			if len(deps) > 0 {
				ecosystems["JS/TS"] = deps
			}
		}

		// 3. Rust (Cargo.toml)
		if content, err := os.ReadFile("Cargo.toml"); err == nil {
			_, deps := parseCargoTOML(string(content))
			if len(deps) > 0 {
				ecosystems["Rust"] = deps
			}
		}

		// 4. Python (requirements.txt)
		if content, err := os.ReadFile("requirements.txt"); err == nil {
			deps := parseRequirementsTXT(string(content))
			if len(deps) > 0 {
				ecosystems["Python"] = deps
			}
		}

		// 5. Ruby (Gemfile)
		if content, err := os.ReadFile("Gemfile"); err == nil {
			deps := parseGemfile(string(content))
			if len(deps) > 0 {
				ecosystems["Ruby"] = deps
			}
		}

		statusParts := make([]string, 0, len(ecosystems))
		// Sort keys for deterministic output ordering
		var ecos []string
		for eco := range ecosystems {
			ecos = append(ecos, eco)
		}
		sort.Strings(ecos)

		for _, eco := range ecos {
			deps := ecosystems[eco]
			statusParts = append(statusParts, fmt.Sprintf("%s: %d deps", eco, len(deps)))
		}

		status := "no dependencies detected"
		if len(statusParts) > 0 {
			status = strings.Join(statusParts, " | ")
		}

		publish(Update{
			Name:    d.Name(),
			Status:  status,
			Payload: ProjectDependencies{Ecosystems: ecosystems},
		})
	}

	// Initial refresh
	refresh()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			refresh()
		case <-events:
			refresh()
		}
	}
}

func parseGoMod(content string) (string, []string) {
	var moduleName string
	var deps []string
	lines := strings.Split(content, "\n")
	inRequire := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "module ") {
			moduleName = strings.TrimSpace(strings.TrimPrefix(line, "module"))
			continue
		}
		if strings.HasPrefix(line, "require ") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "require"))
			if strings.HasPrefix(rest, "(") {
				inRequire = true
			} else {
				parts := strings.Fields(rest)
				if len(parts) > 0 {
					deps = append(deps, parts[0])
				}
			}
			continue
		}
		if inRequire {
			if strings.HasPrefix(line, ")") {
				inRequire = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) > 0 {
				deps = append(deps, parts[0])
			}
		}
	}
	sort.Strings(deps)
	return moduleName, deps
}

func parsePackageJSON(content string) (string, []string) {
	var data struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Name            string            `json:"name"`
	}
	err := json.Unmarshal([]byte(content), &data)
	if err != nil {
		return "", nil
	}
	var deps []string
	for name := range data.Dependencies {
		deps = append(deps, name)
	}
	for name := range data.DevDependencies {
		deps = append(deps, name)
	}
	sort.Strings(deps)
	return data.Name, deps
}

func parseCargoTOML(content string) (string, []string) {
	var packageName string
	var deps []string
	lines := strings.Split(content, "\n")
	inDependencies := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(line[1 : len(line)-1])
			if section == "dependencies" || section == "dev-dependencies" || section == "build-dependencies" {
				inDependencies = true
			} else {
				inDependencies = false
			}
			continue
		}
		if strings.HasPrefix(line, "name =") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "name ="))
			packageName = strings.Trim(val, "\"'")
			continue
		}
		if inDependencies {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) > 0 {
				depName := strings.TrimSpace(parts[0])
				if depName != "" {
					deps = append(deps, depName)
				}
			}
		}
	}
	sort.Strings(deps)
	return packageName, deps
}

func parseRequirementsTXT(content string) []string {
	var deps []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-r") || strings.HasPrefix(line, "-c") || strings.HasPrefix(line, "-e") {
			continue
		}
		idx := strings.IndexAny(line, "=<>!~@;[ ")
		depName := line
		if idx != -1 {
			depName = strings.TrimSpace(line[:idx])
		}
		if depName != "" {
			deps = append(deps, depName)
		}
	}
	sort.Strings(deps)
	return deps
}

func parseGemfile(content string) []string {
	var deps []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "gem ") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "gem"))
			parts := strings.Split(val, ",")
			gemName := strings.TrimSpace(parts[0])
			gemName = strings.Trim(gemName, "\"'")
			if gemName != "" {
				deps = append(deps, gemName)
			}
		}
	}
	sort.Strings(deps)
	return deps
}
