package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillMetadata represents the YAML Front Matter of a Skill.md file
type SkillMetadata struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Version     string    `yaml:"version"`
	Author      string    `yaml:"author"`
	Created     string    `yaml:"created"`
	Updated     string    `yaml:"updated"`
	Tags        []string  `yaml:"tags"`
	Tools       []ToolRef `yaml:"tools"`
}

// ToolRef represents a tool reference in skill metadata
type ToolRef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// SkillInfo contains information about a skill
type SkillInfo struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	SkillPath   string        `json:"skill_path"`
	Content     string        `json:"content"`  // Full Skill.md content (without front matter)
	Metadata    SkillMetadata `json:"metadata"` // Parsed YAML Front Matter
}

// Discovery handles discovering and caching skill descriptions
type Discovery struct {
	skillsDir     string
	cache         map[string]SkillInfo
	cacheHash     string
	lastCheck     time.Time
	mu            sync.RWMutex
	checkInterval time.Duration
}

// NewDiscovery creates a new skill discovery instance
func NewDiscovery(skillsDir string) *Discovery {
	return &Discovery{
		skillsDir:     skillsDir,
		cache:         make(map[string]SkillInfo),
		checkInterval: 5 * time.Second, // Minimum time between directory scans
	}
}

// GetSkills returns all discovered skills with their descriptions
func (d *Discovery) GetSkills(ctx context.Context) ([]SkillInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if cache is still valid (within checkInterval)
	if time.Since(d.lastCheck) < d.checkInterval {
		return d.cachedSkillsList(), nil
	}

	// Directory changed or cache empty, re-discover skills
	d.cacheHash = ""
	d.cache = make(map[string]SkillInfo)

	skills, err := d.discoverSkills(ctx)
	if err != nil {
		// Return partial cache on error
		return d.cachedSkillsList(), nil
	}

	for _, skill := range skills {
		d.cache[skill.Name] = skill
	}

	d.lastCheck = time.Now()
	return d.cachedSkillsList(), nil
}

// cachedSkillsList returns the cached skills as a sorted slice
func (d *Discovery) cachedSkillsList() []SkillInfo {
	result := make([]SkillInfo, 0, len(d.cache))

	// Add discovered skills
	for _, skill := range d.cache {
		result = append(result, skill)
	}

	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// computeDirHash computes a hash of the skills directory structure
func (d *Discovery) computeDirHash() (string, error) {
	var paths []string

	err := filepath.Walk(d.skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		// Only consider Skill.md files for skill directories
		if !info.IsDir() && filepath.Base(path) == "Skill.md" {
			relPath, _ := filepath.Rel(d.skillsDir, path)
			paths = append(paths, relPath)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	// Sort for consistent hashing
	sort.Strings(paths)

	// Compute hash
	hash := sha256.New()
	for _, path := range paths {
		hash.Write([]byte(path))
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// discoverSkills scans the skills directory and extracts skill information
func (d *Discovery) discoverSkills(ctx context.Context) ([]SkillInfo, error) {
	var skills []SkillInfo

	entries, err := os.ReadDir(d.skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Skills directory doesn't exist yet, return empty list
			return []SkillInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip hidden directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		skillPath := filepath.Join(d.skillsDir, entry.Name())

		// Check if Skill.md exists
		skillMdPath := filepath.Join(skillPath, "Skill.md")
		if _, err := os.Stat(skillMdPath); os.IsNotExist(err) {
			continue
		}

		// Read Skill.md content
		content, err := os.ReadFile(skillMdPath)
		if err != nil {
			continue
		}

		// Parse YAML Front Matter
		metadata, bodyContent, err := parseYAMLFrontMatter(string(content))
		if err != nil || strings.TrimSpace(metadata.Description) == "" || strings.TrimSpace(bodyContent) == "" {
			continue
		}

		// Use metadata name if available, otherwise use directory name
		skillName := metadata.Name
		if skillName == "" {
			skillName = entry.Name()
		}

		// Use metadata description if available, otherwise extract from body
		description := metadata.Description
		if description == "" {
			description = extractFirstLine(bodyContent)
		}

		skills = append(skills, SkillInfo{
			Name:        skillName,
			Description: description,
			SkillPath:   skillPath,
			Content:     bodyContent,
			Metadata:    metadata,
		})
	}

	return skills, nil
}

// extractFirstLine extracts the first non-empty line from content
func extractFirstLine(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return ""
}

// parseYAMLFrontMatter parses YAML Front Matter from Skill.md content
// Returns metadata, body content (without front matter), and error
func parseYAMLFrontMatter(content string) (SkillMetadata, string, error) {
	// Check if content starts with YAML Front Matter
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return SkillMetadata{}, content, fmt.Errorf("no YAML Front Matter found")
	}

	// Split content by ---
	parts := strings.SplitN(strings.TrimSpace(content), "---", 3)
	if len(parts) < 3 {
		return SkillMetadata{}, content, fmt.Errorf("invalid YAML Front Matter format")
	}

	// parts[0] should be empty (before first ---)
	// parts[1] should be YAML content
	// parts[2] should be markdown body

	yamlContent := strings.TrimSpace(parts[1])
	bodyContent := strings.TrimSpace(parts[2])

	// Parse YAML
	var metadata SkillMetadata
	if err := yaml.Unmarshal([]byte(yamlContent), &metadata); err != nil {
		return SkillMetadata{}, content, fmt.Errorf("failed to parse YAML Front Matter: %w", err)
	}

	return metadata, bodyContent, nil
}

// ToJSON returns the skill list as JSON for template injection
func (d *Discovery) ToJSON(ctx context.Context) (string, error) {
	skills, err := d.GetSkills(ctx)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(skills)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// SkillNames returns a simple comma-separated list of skill names
func (d *Discovery) SkillNames(ctx context.Context) string {
	skills, err := d.GetSkills(ctx)
	if err != nil {
		return ""
	}

	names := make([]string, len(skills))
	for i, skill := range skills {
		names[i] = skill.Name
	}

	return strings.Join(names, ", ")
}
