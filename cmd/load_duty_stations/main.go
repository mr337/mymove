package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gobuffalo/pop"
	"github.com/namsral/flag"
	"go.uber.org/zap"

	dutyStations "github.com/transcom/mymove/pkg/services/duty_stations"
)

func setup() (*pop.Connection, *zap.Logger) {
	config := flag.String("config-dir", "config", "The location of server config files")
	verbose := flag.Bool("verbose", false, "Sets debug logging level")
	env := flag.String("env", "development", "The environment to run in, which configures the database.")
	validate := flag.Bool("validate", false, "Only run file validations")
	flag.Parse()

	zapConfig := zap.NewDevelopmentConfig()
	logger, _ := zapConfig.Build()

	zapConfig.Level.SetLevel(zap.InfoLevel)
	if *verbose {
		zapConfig.Level.SetLevel(zap.DebugLevel)
	}

	//DB connection
	err := pop.AddLookupPaths(*config)
	if err != nil {
		logger.Panic("Error initializing db connection", zap.Error(err))
	}
	db, err := pop.Connect(*env)
	if err != nil {
		logger.Panic("Error initializing db connection", zap.Error(err))
	}

	// If we just want to validate files we can exit
	if validate != nil && *validate {
		os.Exit(0)
	}
	return db, logger
}

// Command: go run github.com/transcom/mymove/cmd/load_duty_stations
func main() {
	inputFile := "./cmd/load_duty_stations/data/Unit_UnitCity_Zip_v5.xlsx"
	outputFile := "./cmd/load_duty_stations/data/outputfile.sql"

	dbConnection, logger := setup()

	builder := dutyStations.NewMigrationBuilder(dbConnection, logger)
	insertions, err := builder.Build(inputFile)
	if err != nil {
		logger.Panic("Error while building migration", zap.Error(err))
	}

	var migration strings.Builder
	migration.WriteString("-- Migration generated using cmd/load_duty_stations\n")
	migration.WriteString(fmt.Sprintf("-- Duty stations file: %v\n", outputFile))
	migration.WriteString("\n")
	migration.WriteString(insertions)

	f, err := os.Create(outputFile)
	defer f.Close()
	if err != nil {
		log.Panic(err)
	}
	_, err = f.WriteString(migration.String())
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("Complete! Migration written to %v\n", outputFile)
}
