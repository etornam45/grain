package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Generate(dir, name string, prompt PromptFunc) (string, error) {
	old, err := LoadLatestSnapshot(dir)
	if err != nil {
		return "", fmt.Errorf("load latest snapshot: %w", err)
	}
	newSnap := BuildSnapshot()

	changes, err := Diff(old, newSnap, prompt)
	if err != nil {
		return "", fmt.Errorf("diff: %w", err)
	}
	if len(changes) == 0 {
		return "", fmt.Errorf("no schema changes detected")
	}

	var up, down, review strings.Builder
	for _, c := range changes {
		switch {
		case c.Destructive:
			fmt.Fprintf(&review, "-- DESTRUCTIVE (%s): %s\n-- %s\n\n", c.Kind, c.Note, c.UpSQL)
		case c.RequiresNoTx:
			fmt.Fprintf(&review, "-- RUN OUTSIDE THIS MIGRATION (%s): %s\n-- %s\n\n", c.Kind, c.Note, c.UpSQL)
		default:
			fmt.Fprintf(&up, "%s\n", c.UpSQL)
			if c.DownSQL != "" {
				fmt.Fprintf(&down, "%s\n", c.DownSQL)
			}
		}
	}

	version := time.Now().UTC().Format("20060102150405")
	fileName := fmt.Sprintf("%s_%s.sql", version, sanitize(name))
	path := filepath.Join(dir, fileName)

	var content strings.Builder
	content.WriteString("-- +migrate Up\n")
	content.WriteString(up.String())
	content.WriteString("\n-- +migrate Down\n")
	content.WriteString(down.String())
	if review.Len() > 0 {
		content.WriteString("\n-- ---------------------------------------------------------------\n")
		content.WriteString("-- The changes below were detected but NOT included above: they're\n")
		content.WriteString("-- destructive in a way no confirmation resolves, or can't run inside\n")
		content.WriteString("-- this file's transaction. Review each one, then apply by hand or\n")
		content.WriteString("-- move it into its own file.\n")
		content.WriteString("-- ---------------------------------------------------------------\n\n")
		content.WriteString(review.String())
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

func sanitize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.ReplaceAll(name, " ", "_")
}
