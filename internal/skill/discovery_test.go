package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscovery_CreateSkill(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create discovery instance
	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Test creating a skill
	err = discovery.CreateSkill("Test_Skill", "这是一个测试技能", "## 描述\n\n测试内容", false)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Verify skill was created
	skills, err := discovery.GetSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to get skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "Test_Skill" {
		t.Fatalf("Expected skill name 'Test_Skill', got '%s'", skills[0].Name)
	}

	// Verify directory structure
	skillDir := filepath.Join(tempDir, "test_skill")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Fatalf("Skill directory not created: %s", skillDir)
	}

	skillMdPath := filepath.Join(skillDir, "Skill.md")
	if _, err := os.Stat(skillMdPath); os.IsNotExist(err) {
		t.Fatalf("Skill.md file not created: %s", skillMdPath)
	}

	t.Logf("✓ Skill created successfully: %s", skills[0].Name)
}

func TestDiscovery_GetSkillByName(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Create a skill
	err = discovery.CreateSkill("Test_Skill", "Test description", "## Test content", false)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Test getting skill by name
	skill, err := discovery.GetSkillByName(ctx, "Test_Skill")
	if err != nil {
		t.Fatalf("Failed to get skill by name: %v", err)
	}

	if skill.Name != "Test_Skill" {
		t.Fatalf("Expected 'Test_Skill', got '%s'", skill.Name)
	}

	t.Logf("✓ GetSkillByName works correctly")
}

func TestDiscovery_UpdateSkill(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Create a skill
	err = discovery.CreateSkill("Update_Test", "Original description", "## Original content", false)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Update the skill
	err = discovery.UpdateSkill("Update_Test", "Updated description", "## Updated content")
	if err != nil {
		t.Fatalf("Failed to update skill: %v", err)
	}

	// Verify update
	skill, err := discovery.GetSkillByName(ctx, "Update_Test")
	if err != nil {
		t.Fatalf("Failed to get updated skill: %v", err)
	}

	if skill.Metadata.Description != "Updated description" {
		t.Fatalf("Expected updated description, got '%s'", skill.Metadata.Description)
	}

	if skill.Content != "## Updated content" {
		t.Fatalf("Expected updated content, got '%s'", skill.Content)
	}

	t.Logf("✓ UpdateSkill works correctly")
}

func TestDiscovery_DeleteSkill(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Create a skill
	err = discovery.CreateSkill("Delete_Test", "To be deleted", "## Delete me", false)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Delete the skill
	err = discovery.DeleteSkill("Delete_Test")
	if err != nil {
		t.Fatalf("Failed to delete skill: %v", err)
	}

	// Verify deletion
	skills, err := discovery.GetSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to get skills: %v", err)
	}

	if len(skills) != 0 {
		t.Fatalf("Expected 0 skills after deletion, got %d", len(skills))
	}

	t.Logf("✓ DeleteSkill works correctly")
}

func TestDiscovery_ListSkills(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Create multiple skills
	skills := []struct {
		name        string
		description string
		content     string
	}{
		{"Skill_1", "Description 1", "Content 1"},
		{"Skill_2", "Description 2", "Content 2"},
		{"Skill_3", "Description 3", "Content 3"},
	}

	for _, s := range skills {
		err := discovery.CreateSkill(s.name, s.description, s.content, false)
		if err != nil {
			t.Fatalf("Failed to create skill %s: %v", s.name, err)
		}
	}

	// List skills
	list := discovery.ListSkills(ctx)
	if len(list) != 3 {
		t.Fatalf("Expected 3 skills, got %d", len(list))
	}

	t.Logf("✓ ListSkills works correctly, found %d skills", len(list))
	for _, s := range list {
		t.Logf("  - %s: %s", s.Name, s.Description)
	}
}

func TestDiscovery_DuplicateSkill(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)

	// Create a skill
	err = discovery.CreateSkill("Duplicate_Test", "Description", "Content", false)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Try to create duplicate
	err = discovery.CreateSkill("Duplicate_Test", "Another description", "Another content", false)
	if err == nil {
		t.Fatal("Expected error when creating duplicate skill")
	}

	t.Logf("✓ Duplicate skill detection works correctly: %v", err)
}

func TestDiscovery_ImportSkillMd(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Create a temporary Skill.md file
	skillMdContent := `---
name: "Imported_Skill"
description: "An imported skill"
version: "1.0.0"
---

# Imported Skill

## Description
This skill was imported from a Skill.md file.
`
	tmpFile, err := os.CreateTemp("", "skill-*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(skillMdContent); err != nil {
		t.Fatalf("Failed to write skill content: %v", err)
	}
	tmpFile.Close()

	// Import the skill
	err = discovery.ImportSkill(tmpFile.Name(), false, "")
	if err != nil {
		t.Fatalf("Failed to import skill: %v", err)
	}

	// Verify skill was imported
	skills, err := discovery.GetSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to get skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "Imported_Skill" {
		t.Fatalf("Expected skill name 'Imported_Skill', got '%s'", skills[0].Name)
	}

	t.Logf("✓ ImportSkill from Skill.md works correctly")
}

func TestDiscovery_ImportSkillWithScripts(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir, nil)
	ctx := context.Background()

	// Create a skill with scripts
	err = discovery.CreateSkill("Skill_With_Scripts", "Skill with scripts", "## Content", true)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Add a script file
	skillDir := filepath.Join(tempDir, "skill_with_scripts")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create scripts dir: %v", err)
	}

	scriptContent := "#!/usr/bin/env python3\nprint('Hello')"
	scriptPath := filepath.Join(scriptsDir, "hello.py")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Get skills and verify scripts are detected
	skills, err := discovery.GetSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to get skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(skills))
	}

	if !skills[0].HasScripts {
		t.Fatal("Expected skill to have scripts")
	}

	if len(skills[0].Scripts) != 1 {
		t.Fatalf("Expected 1 script, got %d", len(skills[0].Scripts))
	}

	if skills[0].Scripts[0].Name != "hello.py" {
		t.Fatalf("Expected script name 'hello.py', got '%s'", skills[0].Scripts[0].Name)
	}

	t.Logf("✓ Skill with scripts detected correctly")
}
