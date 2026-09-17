package migrate

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type AmbiguityKind string

const (
	AmbiguousTable  AmbiguityKind = "table"
	AmbiguousColumn AmbiguityKind = "column"
)

type PromptContext struct {
	Kind       AmbiguityKind
	Table      string
	OldName    string
	Candidates []string
}

type Resolution struct {
	Action string // "rename", "delete", or "ignore"
	Target string // the new name, when Action == "rename"
}

type PromptFunc func(ctx PromptContext) (Resolution, error)

func CLIPrompt(ctx PromptContext) (Resolution, error) {
	switch ctx.Kind {
	case AmbiguousTable:
		fmt.Printf("\nTable %q is in the last migration but not in your schema code.\n", ctx.OldName)
	case AmbiguousColumn:
		fmt.Printf("\nColumn %q on table %q is in the last migration but not in your schema code.\n", ctx.OldName, ctx.Table)
	}

	for i, c := range ctx.Candidates {
		fmt.Printf("  [%d] renamed to %q\n", i+1, c)
	}
	fmt.Println("  [d] deleted — generate a DROP for it")
	fmt.Println("  [i] ignore — leave it in the database untouched for now")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	if idx, err := strconv.Atoi(line); err == nil && idx >= 1 && idx <= len(ctx.Candidates) {
		return Resolution{Action: "rename", Target: ctx.Candidates[idx-1]}, nil
	}
	switch strings.ToLower(line) {
	case "d", "delete", "deleted":
		return Resolution{Action: "delete"}, nil
	default:
		return Resolution{Action: "ignore"}, nil
	}
}
