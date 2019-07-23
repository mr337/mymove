package main

import (
	"log"
	"os"
	"path/filepath"
	"text/template"

	"github.com/pkg/errors"
)

const (
	// OfficeUsersMigrationFile sql file containing the migration to add the new office users
	OfficeUsersMigrationFilenameFlag string = "migration-filename"

	// VersionTimeFormat is the Go time format for creating a version number.
	VersionTimeFormat string = "20060102150405"

	// secureMigrationTemplate is the template to apply secure migration
	secureMigrationTemplate string = `exec("./apply-secure-migration.sh {{.}}")`

	// tempMigrationPath is the temporary path for generated migrations
	tempMigrationPath string = "./tmp"
)

// Close an open file or exit
func closeFile(outfile *os.File) {
	err := outfile.Close()
	if err != nil {
		log.Printf("error closing %s: %v\n", outfile.Name(), err)
		os.Exit(1)
	}
}

func createMigration(path string, filename string, t *template.Template, templateData interface{}) error {
	migrationPath := filepath.Join(path, filename)
	migrationFile, err := os.Create(migrationPath)
	defer closeFile(migrationFile)
	if err != nil {
		return errors.Wrapf(err, "error creating %s", migrationPath)
	}
	err = t.Execute(migrationFile, templateData)
	if err != nil {
		log.Println("error executing template: ", err)
	}
	log.Printf("new migration file created at:  %q\n", migrationPath)
	return nil
}
