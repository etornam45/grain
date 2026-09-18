package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"grain/db"
	"grain/migrate"
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

		path, err := migrate.GenerateFromSnapshot(newSnap, dir, name, migrate.CLIPrompt)
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
		conn, err := db.Open("pgx", dsn)
		if err != nil {
			fmt.Println("connect:", err)
			os.Exit(1)
		}
		defer conn.Close()
		ctx := context.Background()

		switch os.Args[2] {
		case "up":
			if err := migrate.Apply(ctx, conn.DB, dir); err != nil {
				fmt.Println("migrate up:", err)
				os.Exit(1)
			}
		case "down":
			if err := migrate.RollbackLast(ctx, conn.DB, dir); err != nil {
				fmt.Println("migrate down:", err)
				os.Exit(1)
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
  grain generate -schema <dir> [name]
  grain migrate up                       (requires DATABASE_URL)
  grain migrate down                     (requires DATABASE_URL)`)
}
