package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/etornam45/grain/db"
	"github.com/etornam45/grain/migrate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	const dir = "db/migrations"

	switch os.Args[1] {
	case "generate":
		fs := flag.NewFlagSet("generate", flag.ExitOnError)
		schemaDir := fs.String("schema", "", "directory containing your schema.Table()/schema.Enum() definitions")
		force := fs.Bool("force", false, "write destructive changes (type casts, SET NOT NULL) without requiring manual review")
		yes := fs.Bool("yes", false, "auto-resolve rename/delete prompts (rename to the only candidate, else ignore)")
		fs.Parse(os.Args[2:])

		if *schemaDir == "" {
			fmt.Println("generate: -schema <dir> is required, e.g. -schema ./internal/schema")
			os.Exit(1)
		}
		name := "change"
		if fs.NArg() > 0 {
			name = fs.Arg(0)
		}

		newSnap, err := migrate.LoadSnapshotFromDir(".", *schemaDir)
		if err != nil {
			fmt.Println("load schema:", err)
			os.Exit(1)
		}

		prompt := migrate.CLIPrompt
		if *yes {
			prompt = migrate.AutoResolvePrompt
		}
		path, err := migrate.GenerateFromSnapshotOpts(newSnap, dir, name, prompt, migrate.GenerateOptions{Force: *force})
		if err != nil {
			fmt.Println("generate:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)

	case "migrate":
		if len(os.Args) < 3 {
			usage()
			os.Exit(1)
		}

		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			fmt.Println("DATABASE_URL environment variable is required")
			os.Exit(1)
		}
		ctx := context.Background()
		conn, err := db.Connect(ctx, "pgx", dsn)
		if err != nil {
			fmt.Println("connect:", err)
			os.Exit(1)
		}
		defer conn.Close()

		switch os.Args[2] {
		case "init":
			if err := migrate.Init(ctx, conn.DB, dir); err != nil {
				fmt.Println("migrate init:", err)
				os.Exit(1)
			}
			fmt.Println("initialized", dir)
		case "up":
			if err := migrate.Apply(ctx, conn.DB, dir); err != nil {
				fmt.Println("migrate up:", err)
				os.Exit(1)
			}
		case "down":
			n := 1
			if len(os.Args) > 3 {
				parsed, err := strconv.Atoi(os.Args[3])
				if err != nil || parsed < 1 {
					fmt.Println("migrate down: count must be a positive integer")
					os.Exit(1)
				}
				n = parsed
			}
			if err := migrate.RollbackN(ctx, conn.DB, dir, n); err != nil {
				fmt.Println("migrate down:", err)
				os.Exit(1)
			}
		case "redo":
			if err := migrate.RollbackN(ctx, conn.DB, dir, 1); err != nil {
				fmt.Println("migrate redo (rollback):", err)
				os.Exit(1)
			}
			if err := migrate.Apply(ctx, conn.DB, dir); err != nil {
				fmt.Println("migrate redo (apply):", err)
				os.Exit(1)
			}
		case "status":
			statuses, err := migrate.Status(ctx, conn.DB, dir)
			if err != nil {
				fmt.Println("migrate status:", err)
				os.Exit(1)
			}
			for _, status := range statuses {
				state := "pending"
				if status.Applied {
					state = "applied"
				}
				if !status.ChecksumVerified {
					state += " (checksum mismatch)"
				}
				if status.Orphaned {
					state += " (orphaned)"
				}
				fmt.Printf("%s %d_%s\n", state, status.Version, status.Name)
			}
		default:
			usage()
			os.Exit(1)
		}

	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`usage:
  grain generate -schema <dir> [-force] [-yes] [name]
  grain migrate init                      (requires DATABASE_URL)
  grain migrate up                        (requires DATABASE_URL)
  grain migrate down [n]                  (requires DATABASE_URL)
  grain migrate redo                      (requires DATABASE_URL)
  grain migrate status                    (requires DATABASE_URL)`)
}