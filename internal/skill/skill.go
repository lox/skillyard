package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Path        string         `json:"path"`
	RelPath     string         `json:"rel_path"`
	Frontmatter map[string]any `json:"frontmatter"`
	Warnings    []Finding      `json:"warnings,omitempty"`
}

type Inspection struct {
	Skill    Skill     `json:"skill"`
	Findings []Finding `json:"findings,omitempty"`
}

type Finding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

func Discover(root string) ([]Skill, error) {
	root = filepath.Clean(root)
	candidates, err := candidates(root)
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, candidate := range candidates {
		s, err := Parse(root, candidate)
		if err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills, nil
}

func Inspect(root string) ([]Inspection, error) {
	root = filepath.Clean(root)
	candidates, err := candidates(root)
	if err != nil {
		return nil, err
	}
	var out []Inspection
	for _, candidate := range candidates {
		s, err := Parse(root, candidate)
		if err != nil {
			rel, relErr := filepath.Rel(root, candidate)
			if relErr != nil {
				rel = filepath.Base(candidate)
			}
			out = append(out, Inspection{
				Skill: Skill{
					Name:     filepath.Base(candidate),
					Path:     filepath.Clean(candidate),
					RelPath:  filepath.ToSlash(rel),
					Warnings: warnings(candidate),
				},
				Findings: []Finding{{
					Code:    "invalid-skill",
					Message: err.Error(),
					Path:    filepath.Join(candidate, "SKILL.md"),
				}},
			})
			continue
		}
		out = append(out, Inspection{
			Skill:    s,
			Findings: Validate(s),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Skill.Name != out[j].Skill.Name {
			return out[i].Skill.Name < out[j].Skill.Name
		}
		return out[i].Skill.RelPath < out[j].Skill.RelPath
	})
	return out, nil
}

func candidates(root string) ([]string, error) {
	var out []string
	if hasSkillMD(root) {
		return []string{root}, nil
	}
	containers := []string{root}
	if info, err := os.Stat(filepath.Join(root, "skills")); err == nil && info.IsDir() {
		containers = append(containers, filepath.Join(root, "skills"))
	}
	for _, container := range containers {
		entries, err := os.ReadDir(container)
		if err != nil {
			return nil, fmt.Errorf("read skill root %s: %w", container, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(container, name)
			if hasSkillMD(path) {
				out = append(out, path)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func hasSkillMD(path string) bool {
	info, err := os.Stat(filepath.Join(path, "SKILL.md"))
	return err == nil && !info.IsDir()
}

func Parse(sourceRoot, path string) (Skill, error) {
	data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("read %s: %w", filepath.Join(path, "SKILL.md"), err)
	}
	fm, err := frontmatter(data)
	if err != nil {
		return Skill{}, fmt.Errorf("%s: %w", filepath.Join(path, "SKILL.md"), err)
	}
	name, _ := fm["name"].(string)
	description, _ := fm["description"].(string)
	rel, err := filepath.Rel(sourceRoot, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	s := Skill{
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		Path:        filepath.Clean(path),
		RelPath:     filepath.ToSlash(rel),
		Frontmatter: fm,
	}
	s.Warnings = warnings(path)
	return s, nil
}

func Validate(s Skill) []Finding {
	var findings []Finding
	base := filepath.Base(s.Path)
	if s.Name == "" {
		findings = append(findings, Finding{Code: "missing-name", Message: "SKILL.md frontmatter must include name", Path: s.Path})
	}
	if s.Description == "" {
		findings = append(findings, Finding{Code: "missing-description", Message: "SKILL.md frontmatter must include description", Path: s.Path})
	}
	if s.Name != "" && s.Name != base {
		findings = append(findings, Finding{Code: "name-mismatch", Message: fmt.Sprintf("skill name %q must match directory %q", s.Name, base), Path: s.Path})
	}
	return findings
}

func frontmatter(data []byte) (map[string]any, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return nil, fmt.Errorf("missing YAML frontmatter")
	}
	lines := bytes.Split(data, []byte("\n"))
	var end int
	for i := 1; i < len(lines); i++ {
		if bytes.Equal(bytes.TrimSpace(lines[i]), []byte("---")) {
			end = i
			break
		}
	}
	if end == 0 {
		return nil, fmt.Errorf("unterminated YAML frontmatter")
	}
	raw := bytes.Join(lines[1:end], []byte("\n"))
	var out map[string]any
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func warnings(path string) []Finding {
	var findings []Finding
	if info, err := os.Stat(filepath.Join(path, "mcp.json")); err == nil && !info.IsDir() {
		findings = append(findings, Finding{Code: "has-mcp", Message: "skill contains mcp.json", Path: filepath.Join(path, "mcp.json")})
	}
	if info, err := os.Stat(filepath.Join(path, "scripts")); err == nil && info.IsDir() {
		findings = append(findings, Finding{Code: "has-scripts", Message: "skill contains scripts/", Path: filepath.Join(path, "scripts")})
	}
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&0o111 != 0 {
			findings = append(findings, Finding{Code: "has-executable", Message: "skill contains executable file", Path: p})
		}
		return nil
	})
	return findings
}

func ByName(skills []Skill) map[string]Skill {
	out := map[string]Skill{}
	for _, s := range skills {
		out[s.Name] = s
	}
	return out
}
