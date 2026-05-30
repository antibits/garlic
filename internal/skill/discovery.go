package skill

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	Enabled     bool          `json:"enabled"`
	SkillPath   string        `json:"skill_path"`
	Content     string        `json:"content"`  // Full Skill.md content (without front matter)
	Metadata    SkillMetadata `json:"metadata"` // Parsed YAML Front Matter
	HasScripts  bool          `json:"has_scripts"`
	Scripts     []ScriptInfo  `json:"scripts,omitempty"`
}

// ScriptInfo contains information about a script file in scripts/ directory
type ScriptInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Discovery handles discovering and caching skill descriptions
type Discovery struct {
	skillsDir      string
	cache          map[string]SkillInfo
	cacheHash      string
	lastCheck      time.Time
	mu             sync.RWMutex
	checkInterval  time.Duration
	disabledSkills []string
}

// NewDiscovery creates a new skill discovery instance
func NewDiscovery(skillsDir string, disabledSkills []string) *Discovery {
	return &Discovery{
		skillsDir:      skillsDir,
		cache:          make(map[string]SkillInfo),
		checkInterval:  5 * time.Second, // Minimum time between directory scans
		disabledSkills: disabledSkills,
	}
}

// UpdateDisabledSkills updates the disabled skills list
func (d *Discovery) UpdateDisabledSkills(disabledSkills []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.disabledSkills = disabledSkills
}

// isSkillDisabled checks if a skill is in the disabled list
func (d *Discovery) isSkillDisabled(name string) bool {
	for _, disabled := range d.disabledSkills {
		if disabled == name {
			return true
		}
	}
	return false
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
		skill.Enabled = !d.isSkillDisabled(skill.Name)
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

		// Check for scripts directory
		scriptsDir := filepath.Join(skillPath, "scripts")
		var scripts []ScriptInfo
		hasScripts := false

		if scriptEntries, err := os.ReadDir(scriptsDir); err == nil {
			hasScripts = true
			for _, scriptEntry := range scriptEntries {
				if scriptEntry.IsDir() || strings.HasPrefix(scriptEntry.Name(), ".") {
					continue
				}
				scriptPath := filepath.Join(scriptsDir, scriptEntry.Name())
				scripts = append(scripts, ScriptInfo{
					Name: scriptEntry.Name(),
					Path: scriptPath,
				})
			}
		}

		skills = append(skills, SkillInfo{
			Name:        skillName,
			Description: description,
			Enabled:     !d.isSkillDisabled(skillName),
			SkillPath:   skillPath,
			Content:     bodyContent,
			Metadata:    metadata,
			HasScripts:  hasScripts,
			Scripts:     scripts,
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

// GetSkillByName returns a skill by name
func (d *Discovery) GetSkillByName(ctx context.Context, name string) (*SkillInfo, error) {
	skills, err := d.GetSkills(ctx)
	if err != nil {
		return nil, err
	}

	for _, skill := range skills {
		if skill.Name == name {
			return &skill, nil
		}
	}

	return nil, fmt.Errorf("skill '%s' not found", name)
}

// CreateSkill creates a new skill directory and Skill.md file
// If withScripts is true, also creates an empty scripts/ directory
func (d *Discovery) CreateSkill(name, description, content string, withScripts bool) error {
	// Validate inputs
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("skill name cannot be empty")
	}
	if !isValidFunctionName(name) {
		return fmt.Errorf("invalid skill name '%s': must contain only letters (a-z, A-Z), digits (0-9), underscores (_), and hyphens (-)", name)
	}
	if strings.TrimSpace(description) == "" {
		return fmt.Errorf("skill description cannot be empty")
	}

	// Sanitize skill name for directory
	skillDir := sanitizeDirName(name)
	skillPath := filepath.Join(d.skillsDir, skillDir)

	// Check if skill already exists
	if _, err := os.Stat(skillPath); err == nil {
		return fmt.Errorf("skill '%s' already exists", name)
	}

	// Create skill directory
	if err := os.MkdirAll(skillPath, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Create scripts directory if requested
	if withScripts {
		scriptsDir := filepath.Join(skillPath, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			// Clean up on error
			os.RemoveAll(skillPath)
			return fmt.Errorf("failed to create scripts directory: %w", err)
		}
	}

	// Generate Skill.md content with YAML Front Matter
	skillContent := generateSkillMarkdown(name, description, content)

	// Write Skill.md file
	skillMdPath := filepath.Join(skillPath, "Skill.md")
	if err := os.WriteFile(skillMdPath, []byte(skillContent), 0644); err != nil {
		// Clean up directory on error
		os.RemoveAll(skillPath)
		return fmt.Errorf("failed to write Skill.md: %w", err)
	}

	// Invalidate cache
	d.mu.Lock()
	d.cache = make(map[string]SkillInfo)
	d.lastCheck = time.Time{}
	d.mu.Unlock()

	return nil
}

// UpdateSkill updates an existing skill's content
func (d *Discovery) UpdateSkill(name, description, content string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("skill name cannot be empty")
	}

	// Find existing skill
	skillInfo, err := d.GetSkillByName(context.Background(), name)
	if err != nil {
		return err
	}

	// Use provided description or keep existing
	if strings.TrimSpace(description) == "" {
		description = skillInfo.Metadata.Description
	}

	// Generate updated Skill.md content
	skillContent := generateSkillMarkdown(name, description, content)

	// Write updated Skill.md file
	skillMdPath := filepath.Join(skillInfo.SkillPath, "Skill.md")
	if err := os.WriteFile(skillMdPath, []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("failed to update Skill.md: %w", err)
	}

	// Invalidate cache
	d.mu.Lock()
	d.cache = make(map[string]SkillInfo)
	d.lastCheck = time.Time{}
	d.mu.Unlock()

	return nil
}

// ImportSkill imports a skill from a Skill.md file or a zip archive
// If isZip is true, sourcePath is treated as a zip file; otherwise as a Skill.md file
// If skillID is provided, it will be used as the directory name; otherwise extracted from file
func (d *Discovery) ImportSkill(sourcePath string, isZip bool, skillID string) error {
	if strings.TrimSpace(sourcePath) == "" {
		return fmt.Errorf("source path cannot be empty")
	}

	// Check if source file exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file '%s' does not exist", sourcePath)
	}

	if isZip {
		return d.importFromZip(sourcePath, skillID)
	}
	return d.importFromSkillMd(sourcePath, skillID)
}

// importFromZip imports a skill from a zip archive
// The zip should contain Skill.md and optionally a scripts/ directory
// If skillID is provided, it will be used as the directory name
func (d *Discovery) importFromZip(zipPath string, skillID string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	// Find Skill.md in the zip
	var skillMdFile *zip.File
	var scriptFiles []*zip.File
	var skillName string

	for _, f := range r.File {
		// Skip directories
		if f.FileInfo().IsDir() {
			// Extract skill name from directory name if possible
			if skillName == "" && strings.HasSuffix(f.Name, "/") {
				skillName = strings.TrimSuffix(f.Name, "/")
				skillName = filepath.Base(skillName)
			}
			continue
		}

		baseName := filepath.Base(f.Name)
		dirName := filepath.Dir(f.Name)

		// Check if this is Skill.md (could be at root or in a subdirectory)
		if baseName == "Skill.md" {
			skillMdFile = f
			if skillName == "" {
				skillName = strings.TrimSuffix(dirName, ".")
			}
			continue
		}

		// Check if this is a script file
		if strings.HasPrefix(f.Name, "scripts/") || strings.HasPrefix(f.Name, "scripts\\") {
			scriptFiles = append(scriptFiles, f)
		}
	}

	if skillMdFile == nil {
		return fmt.Errorf("zip file does not contain Skill.md")
	}

	// Use directory name from zip or generate from Skill.md
	if skillName == "" || skillName == "." {
		// Try to extract name from Skill.md content
		name, err := extractSkillNameFromZipFile(skillMdFile)
		if err != nil {
			return fmt.Errorf("failed to extract skill name: %w", err)
		}
		skillName = name
	}

	// If skillID is provided, use it as the directory name
	if strings.TrimSpace(skillID) != "" {
		skillName = skillID
	}

	// Validate skill name for API compatibility
	if !isValidFunctionName(skillName) {
		return fmt.Errorf("invalid skill name '%s': must contain only letters (a-z, A-Z), digits (0-9), underscores (_), and hyphens (-)", skillName)
	}

	// Sanitize skill name for directory
	skillDir := sanitizeDirName(skillName)
	skillPath := filepath.Join(d.skillsDir, skillDir)

	// Check if skill already exists
	if _, err := os.Stat(skillPath); err == nil {
		return fmt.Errorf("skill '%s' already exists", skillName)
	}

	// Create skill directory
	if err := os.MkdirAll(skillPath, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Extract Skill.md
	skillMdPath := filepath.Join(skillPath, "Skill.md")
	if err := extractZipFile(skillMdFile, skillMdPath); err != nil {
		os.RemoveAll(skillPath)
		return fmt.Errorf("failed to extract Skill.md: %w", err)
	}

	// Extract scripts if any
	if len(scriptFiles) > 0 {
		scriptsDir := filepath.Join(skillPath, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			os.RemoveAll(skillPath)
			return fmt.Errorf("failed to create scripts directory: %w", err)
		}

		for _, scriptFile := range scriptFiles {
			scriptName := filepath.Base(scriptFile.Name)
			scriptPath := filepath.Join(scriptsDir, scriptName)
			if err := extractZipFile(scriptFile, scriptPath); err != nil {
				os.RemoveAll(skillPath)
				return fmt.Errorf("failed to extract script '%s': %w", scriptName, err)
			}
		}
	}

	// Invalidate cache
	d.mu.Lock()
	d.cache = make(map[string]SkillInfo)
	d.lastCheck = time.Time{}
	d.mu.Unlock()

	return nil
}

// importFromSkillMd imports a skill from a single Skill.md file
// If skillID is provided, it will be used as the directory name
func (d *Discovery) importFromSkillMd(skillMdPath string, skillID string) error {
	// Read Skill.md content
	content, err := os.ReadFile(skillMdPath)
	if err != nil {
		return fmt.Errorf("failed to read Skill.md: %w", err)
	}

	// Parse YAML Front Matter to get skill name
	metadata, _, err := parseYAMLFrontMatter(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse Skill.md front matter: %w", err)
	}

	// Use metadata name or generate from filename
	skillName := metadata.Name
	if skillName == "" {
		baseName := filepath.Base(skillMdPath)
		skillName = strings.TrimSuffix(baseName, ".md")
	}

	// If skillID is provided, use it as the directory name
	if strings.TrimSpace(skillID) != "" {
		skillName = skillID
	}

	// Validate skill name for API compatibility
	if !isValidFunctionName(skillName) {
		return fmt.Errorf("invalid skill name '%s': must contain only letters (a-z, A-Z), digits (0-9), underscores (_), and hyphens (-)", skillName)
	}

	// Sanitize skill name for directory
	skillDir := sanitizeDirName(skillName)
	skillPath := filepath.Join(d.skillsDir, skillDir)

	// Check if skill already exists
	if _, err := os.Stat(skillPath); err == nil {
		return fmt.Errorf("skill '%s' already exists", skillName)
	}

	// Create skill directory
	if err := os.MkdirAll(skillPath, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Copy Skill.md to skill directory
	destPath := filepath.Join(skillPath, "Skill.md")
	if err := copyFile(skillMdPath, destPath); err != nil {
		os.RemoveAll(skillPath)
		return fmt.Errorf("failed to copy Skill.md: %w", err)
	}

	// Invalidate cache
	d.mu.Lock()
	d.cache = make(map[string]SkillInfo)
	d.lastCheck = time.Time{}
	d.mu.Unlock()

	return nil
}

// extractSkillNameFromZipFile extracts skill name from a zip file's Skill.md
func extractSkillNameFromZipFile(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	metadata, _, err := parseYAMLFrontMatter(string(content))
	if err != nil || metadata.Name == "" {
		return "", fmt.Errorf("could not extract skill name from front matter")
	}

	return metadata.Name, nil
}

// extractZipFile extracts a single file from a zip archive
func extractZipFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// DeleteSkill deletes a skill directory and all its contents
func (d *Discovery) DeleteSkill(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("skill name cannot be empty")
	}

	// Find existing skill
	skillInfo, err := d.GetSkillByName(context.Background(), name)
	if err != nil {
		return err
	}

	// Remove skill directory
	if err := os.RemoveAll(skillInfo.SkillPath); err != nil {
		return fmt.Errorf("failed to delete skill directory: %w", err)
	}

	// Invalidate cache
	d.mu.Lock()
	d.cache = make(map[string]SkillInfo)
	d.lastCheck = time.Time{}
	d.mu.Unlock()

	return nil
}

// ListSkills returns a simple list of skill names and descriptions
func (d *Discovery) ListSkills(ctx context.Context) []struct {
	Name        string
	Description string
	Path        string
	HasScripts  bool
	Scripts     []ScriptInfo
} {
	skills, err := d.GetSkills(ctx)
	if err != nil {
		return nil
	}

	result := make([]struct {
		Name        string
		Description string
		Path        string
		HasScripts  bool
		Scripts     []ScriptInfo
	}, len(skills))

	for i, skill := range skills {
		result[i] = struct {
			Name        string
			Description string
			Path        string
			HasScripts  bool
			Scripts     []ScriptInfo
		}{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.SkillPath,
			HasScripts:  skill.HasScripts,
			Scripts:     skill.Scripts,
		}
	}

	return result
}

// sanitizeDirName converts a skill name to a valid directory name
func sanitizeDirName(name string) string {
	// Replace spaces and special characters with underscores
	result := strings.ToLower(name)
	result = strings.ReplaceAll(result, " ", "_")
	result = strings.ReplaceAll(result, "-", "_")

	// Keep alphanumeric, underscore, and non-ASCII characters (like Chinese)
	var sanitized strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r >= 128 {
			sanitized.WriteRune(r)
		}
	}

	return sanitized.String()
}

// isValidFunctionName checks if a name is valid for use as an OpenAI function call name.
// Must match ^[a-zA-Z0-9_-]+$ per API requirements (enforced by DeepSeek and others).
func isValidFunctionName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// generateSkillMarkdown generates a complete Skill.md file with YAML Front Matter
func generateSkillMarkdown(name, description, content string) string {
	now := time.Now().Format("2006-01-02")

	// If content doesn't start with #, add a title
	if !strings.HasPrefix(strings.TrimSpace(content), "#") {
		content = fmt.Sprintf("# %s\n\n%s", name, content)
	}

	metadata := fmt.Sprintf(`---
name: "%s"
description: "%s"
version: "1.0.0"
author: "Garlic Team"
created: "%s"
updated: "%s"
tags: []
tools: []
---

%s`, name, description, now, now, content)

	return metadata
}
