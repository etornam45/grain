package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type GenerateOptions struct {
	Force bool
}

func GenerateFromSnapshot(newSnap Snapshot, dir, name string, prompt PromptFunc) (string, error) {
	return generate(newSnap, dir, name, prompt, GenerateOptions{})
}

func GenerateFromSnapshotOpts(newSnap Snapshot, dir, name string, prompt PromptFunc, opts GenerateOptions) (string, error) {
	return generate(newSnap, dir, name, prompt, opts)
}

func generate(newSnap Snapshot, dir, name string, prompt PromptFunc, opts GenerateOptions) (string, error) {
	old, err := LoadLatestSnapshot(dir)
	if err != nil {
		return "", fmt.Errorf("load latest snapshot: %w", err)
	}

	changes, err := Diff(old, newSnap, prompt)
	if err != nil {
		return "", fmt.Errorf("diff: %w", err)
	}
	if len(changes) == 0 {
		return "", fmt.Errorf("no schema changes detected")
	}
	if !opts.Force {
		for _, change := range changes {
			if change.Destructive {
				return "", &ManualReviewRequiredError{Changes: changes}
			}
		}
	}

	var up, noTxUp, down, noTxDown strings.Builder
	for _, c := range changes {
		if c.RequiresNoTx {
			fmt.Fprintf(&noTxUp, "%s\n", c.UpSQL)
		} else {
			fmt.Fprintf(&up, "%s\n", c.UpSQL)
		}
	}
	for i := len(changes) - 1; i >= 0; i-- {
		if changes[i].DownSQL == "" {
			continue
		}
		if changes[i].RequiresNoTx {
			fmt.Fprintf(&noTxDown, "%s\n", changes[i].DownSQL)
		} else {
			fmt.Fprintf(&down, "%s\n", changes[i].DownSQL)
		}
	}

	version, err := nextMigrationVersion(dir, time.Now().UTC())
	if err != nil {
		return "", err
	}
	fileName := fmt.Sprintf("%s_%s.sql", version, sanitize(name))
	path := filepath.Join(dir, fileName)

	var content strings.Builder
	if noTxUp.Len() > 0 {
		content.WriteString("-- +RequiresNoTx Up\n")
		content.WriteString(noTxUp.String())
		content.WriteString("\n")
	}
	content.WriteString("-- +migrate Up\n")
	content.WriteString(up.String())
	content.WriteString("\n-- +migrate Down\n")
	content.WriteString(down.String())
	if noTxDown.Len() > 0 {
		content.WriteString("\n-- +RequiresNoTx Down\n")
		content.WriteString(noTxDown.String())
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		return "", err
	}

	if err := SaveSnapshot(dir, version, name, newSnap); err != nil {
		return "", fmt.Errorf("save snapshot (migration file was written, but the snapshot journal was not): %w", err)
	}

	return path, nil
}

func nextMigrationVersion(dir string, now time.Time) (string, error) {
	base := now.UTC().Format("20060102150405000")
	for suffix := 0; ; suffix++ {
		version := base
		if suffix > 0 {
			version = fmt.Sprintf("%s%02d", base, suffix)
		}
		if _, err := os.Stat(filepath.Join(dir, "meta", version+"_snapshot.json")); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		matches, err := filepath.Glob(filepath.Join(dir, version+"_*.sql"))
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			if _, err := os.Stat(filepath.Join(dir, "meta", version+"_snapshot.json")); os.IsNotExist(err) {
				return version, nil
			} else if err != nil {
				return "", err
			}
		}
	}
}

type ManualReviewRequiredError struct {
	Changes []Change
}

func (e *ManualReviewRequiredError) Error() string {
	var details []string
	for _, change := range e.Changes {
		if change.Destructive {
			details = append(details, fmt.Sprintf("%s: %s", change.Kind, change.Note))
		}
	}
	return "manual migration required before updating the schema snapshot: " + strings.Join(details, "; ")
}

func sanitize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.ReplaceAll(name, " ", "_")
}
