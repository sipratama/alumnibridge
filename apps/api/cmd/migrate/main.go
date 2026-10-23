package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	urlpkg "net/url"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	path := flag.String("path", "../../infrastructure/migrations", "migration directory")
	flag.Parse()
	if flag.NArg() != 1 || flag.Arg(0) != "up" {
		log.Fatal("usage: go run ./cmd/migrate [-path DIR] up")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	abs, err := filepath.Abs(*path)
	if err != nil {
		log.Fatal(err)
	}
	// The file source reads URL opaque paths directly, including Windows drive letters.
	sourceURL := urlpkg.URL{Scheme: "file", Opaque: filepath.ToSlash(abs)}
	m, err := migrate.New(sourceURL.String(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("migrations already current")
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("migrations applied")
}
