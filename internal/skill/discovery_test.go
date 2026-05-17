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
	discovery := NewDiscovery(tempDir)
	ctx := context.Background()

	// Test creating a skill
	err = discovery.CreateSkill("测试技能", "这是一个测试技能", "## 描述\n\n测试内容")
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

	if skills[0].Name != "测试技能" {
		t.Fatalf("Expected skill name '测试技能', got '%s'", skills[0].Name)
	}

	// Verify directory structure
	skillDir := filepath.Join(tempDir, "测试技能")
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

	discovery := NewDiscovery(tempDir)
	ctx := context.Background()

	// Create a skill
	err = discovery.CreateSkill("Test Skill", "Test description", "## Test content")
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Test getting skill by name
	skill, err := discovery.GetSkillByName(ctx, "Test Skill")
	if err != nil {
		t.Fatalf("Failed to get skill by name: %v", err)
	}

	if skill.Name != "Test Skill" {
		t.Fatalf("Expected 'Test Skill', got '%s'", skill.Name)
	}

	t.Logf("✓ GetSkillByName works correctly")
}

func TestDiscovery_UpdateSkill(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "skill_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discovery := NewDiscovery(tempDir)
	ctx := context.Background()

	// Create a skill
	err = discovery.CreateSkill("Update Test", "Original description", "## Original content")
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Update the skill
	err = discovery.UpdateSkill("Update Test", "Updated description", "## Updated content")
	if err != nil {
		t.Fatalf("Failed to update skill: %v", err)
	}

	// Verify update
	skill, err := discovery.GetSkillByName(ctx, "Update Test")
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

	discovery := NewDiscovery(tempDir)
	ctx := context.Background()

	// Create a skill
	err = discovery.CreateSkill("Delete Test", "To be deleted", "## Delete me")
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Delete the skill
	err = discovery.DeleteSkill("Delete Test")
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

	discovery := NewDiscovery(tempDir)
	ctx := context.Background()

	// Create multiple skills
	skills := []struct {
		name        string
		description string
		content     string
	}{
		{"Skill 1", "Description 1", "Content 1"},
		{"Skill 2", "Description 2", "Content 2"},
		{"Skill 3", "Description 3", "Content 3"},
	}

	for _, s := range skills {
		err := discovery.CreateSkill(s.name, s.description, s.content)
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

	discovery := NewDiscovery(tempDir)

	// Create a skill
	err = discovery.CreateSkill("Duplicate Test", "Description", "Content")
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Try to create duplicate
	err = discovery.CreateSkill("Duplicate Test", "Another description", "Another content")
	if err == nil {
		t.Fatal("Expected error when creating duplicate skill")
	}

	t.Logf("✓ Duplicate skill detection works correctly: %v", err)
}
