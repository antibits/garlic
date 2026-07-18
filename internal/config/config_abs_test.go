package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolsDirResolvedAbsolute(t *testing.T) {
	// Write a minimal config into a temp dir, then load it by absolute path
	// while the process CWD is irrelevant (config dir anchors resolution).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "tools:\n  tools_dir: tools\n  skills_dir: skills\n  python_path: python3\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	wantTools := filepath.Join(dir, "tools")
	if cfg.Tools.ToolsDir != wantTools {
		t.Errorf("ToolsDir = %q, want %q", cfg.Tools.ToolsDir, wantTools)
	}
	wantSkills := filepath.Join(dir, "skills")
	if cfg.Tools.SkillsDir != wantSkills {
		t.Errorf("SkillsDir = %q, want %q", cfg.Tools.SkillsDir, wantSkills)
	}
	// python_path is left as a PATH command, not made absolute
	if cfg.Tools.PythonPath != "python3" {
		t.Errorf("PythonPath = %q, want python3", cfg.Tools.PythonPath)
	}
}
