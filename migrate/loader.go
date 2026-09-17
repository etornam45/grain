package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func LoadSnapshotFromPackage(moduleDir, schemaImportPath string) (Snapshot, error) {
	absModuleDir, err := filepath.Abs(moduleDir)
	if err != nil {
		return Snapshot{}, err
	}

	tmpDir, err := os.MkdirTemp(absModuleDir, ".grain-loader-*")
	if err != nil {
		return Snapshot{}, fmt.Errorf("create loader dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	mainGo := fmt.Sprintf(`
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"grain/migrate"
	_ %q
)

func main() {
	snap := migrate.BuildSnapshot()
	if err := json.NewEncoder(os.Stdout).Encode(snap); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`, schemaImportPath)

	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainGo), 0o644); err != nil {
		return Snapshot{}, fmt.Errorf("write loader program: %w", err)
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = tmpDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return Snapshot{}, fmt.Errorf(
			"run schema loader for %q: %w\n%s", schemaImportPath, err, stderr.String(),
		)
	}

	var snap Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		return Snapshot{}, fmt.Errorf("parse loader output: %w", err)
	}

	return snap, nil
}

func LoadSnapshotFromDir(moduleDir, schemaDir string) (Snapshot, error) {
	importPath, err := ImportPathForDir(moduleDir, schemaDir)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve import path for %s: %w", schemaDir, err)
	}
	return LoadSnapshotFromPackage(moduleDir, importPath)
}

func ImportPathForDir(moduleDir, targetDir string) (string, error) {
	modPath, err := moduleImportPath(moduleDir)
	if err != nil {
		return "", err
	}

	absModuleDir, err := filepath.Abs(moduleDir)
	if err != nil {
		return "", err
	}
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absModuleDir, absTargetDir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return modPath, nil
	}
	return modPath + "/" + filepath.ToSlash(rel), nil
}

func moduleImportPath(moduleDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module directive found in %s/go.mod", moduleDir)
}
