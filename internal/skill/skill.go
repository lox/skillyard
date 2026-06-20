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

type DiscoveryOptions struct {
	FullDepth bool
}

type Finding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

func Discover(root string) ([]Skill, error) {
	return DiscoverWithOptions(root, DiscoveryOptions{})
}

func DiscoverWithOptions(root string, opts DiscoveryOptions) ([]Skill, error) {
	root = filepath.Clean(root)
	candidates, err := candidates(root, opts)
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
	return InspectWithOptions(root, DiscoveryOptions{})
}

func InspectWithOptions(root string, opts DiscoveryOptions) ([]Inspection, error) {
	root = filepath.Clean(root)
	candidates, err := candidates(root, opts)
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

func candidates(root string, opts DiscoveryOptions) ([]string, error) {
	if opts.FullDepth {
		return recursiveCandidates(root)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("read skill root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("read skill root %s: not a directory", root)
	}
	var out []string
	if hasSkillMD(root) {
		return []string{root}, nil
	}
	seen := map[string]bool{}
	for _, container := range []string{
		root,
		filepath.Join(root, "skills"),
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".claude", "skills"),
	} {
		candidates, err := childSkillCandidates(container)
		if err != nil {
			return nil, err
		}
		appendCandidates(&out, seen, candidates)
	}
	nested, err := nestedSkillCandidates(filepath.Join(root, "skills"))
	if err != nil {
		return nil, err
	}
	appendCandidates(&out, seen, nested)
	sort.Strings(out)
	return out, nil
}

func childSkillCandidates(container string) ([]string, error) {
	entries, err := os.ReadDir(container)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill root %s: %w", container, err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !entry.IsDir() {
			continue
		}
		path := filepath.Join(container, name)
		if hasSkillMD(path) {
			out = append(out, path)
		}
	}
	return out, nil
}

func nestedSkillCandidates(container string) ([]string, error) {
	entries, err := os.ReadDir(container)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill root %s: %w", container, err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !entry.IsDir() {
			continue
		}
		candidates, err := childSkillCandidates(filepath.Join(container, name))
		if err != nil {
			return nil, err
		}
		out = append(out, candidates...)
	}
	return out, nil
}

func recursiveCandidates(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("walk skill root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("walk skill root %s: not a directory", root)
	}
	var out []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if path != root && (name == ".git" || name == "node_modules") {
			return filepath.SkipDir
		}
		if hasSkillMD(path) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk skill root %s: %w", root, err)
	}
	sort.Strings(out)
	return out, nil
}

func appendCandidates(out *[]string, seen map[string]bool, candidates []string) {
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		*out = append(*out, candidate)
	}
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
