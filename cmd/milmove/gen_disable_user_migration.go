package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/spf13/viper"

	"github.com/transcom/mymove/pkg/cli"
)

const (
	DisableUserEmailFlag string = "migration-email"
)

const (
	// template for adding office users
	disableUser string = `UPDATE users
SET disabled=true
WHERE login_gov_email='{{.EmailPrefix}}@{{.EmailDomain}}';

UPDATE admin_users
SET disabled=true
WHERE email='{{.EmailPrefix}}@{{.EmailDomain}}';

UPDATE office_users
SET disabled=true
WHERE email='{{.EmailPrefix}}@{{.EmailDomain}}';

UPDATE tsp_users
SET disabled=true
WHERE email='{{.EmailPrefix}}+pyvl@{{.EmailDomain}}'
	OR email='{{.EmailPrefix}}+dlxm@{{.EmailDomain}}'
	OR email='{{.EmailPrefix}}+ssow@{{.EmailDomain}}';
`
)

func InitDisableUserFlags(flag *pflag.FlagSet) {
	flag.StringP(DisableUserEmailFlag, "e", "", "Email address")
}

func initDisableUserMigrationFlags(flag *pflag.FlagSet) {
	// Migration Config
	cli.InitMigrationFlags(flag)

	// Migration File Config
	cli.InitMigrationFileFlags(flag)

	// Disable User
	InitDisableUserFlags(flag)

	// Don't sort command line flags
	flag.SortFlags = false
}

// UserTemplate is a struct that stores the EmailPrefix from which to generate the migration
type UserTemplate struct {
	EmailPrefix string
	EmailDomain string
}

func CheckDisableUserFlags(v *viper.Viper) error {
	if err := cli.CheckMigration(v); err != nil {
		return err
	}

	if err := cli.CheckMigrationFile(v); err != nil {
		return err
	}

	email := v.GetString(DisableUserEmailFlag)
	if len(email) == 0 {
		return errors.Errorf("%s is missing", DisableUserEmailFlag)
	}
	return nil
}

func genDisableUserMigration(cmd *cobra.Command, args []string) error {
	err := cmd.ParseFlags(args)
	flag := cmd.Flags()
	err = flag.Parse(os.Args[1:])
	if err != nil {
		return errors.Wrap(err, "could not parse flags")
	}
	v := viper.New()
	err = v.BindPFlags(flag)
	if err != nil {
		return errors.Wrap(err, "could not bind flags")
	}

	migrationsPath := v.GetString(cli.MigrationPathFlag)
	migrationManifest := v.GetString(cli.MigrationManifestFlag)
	// migrationName := v.GetString(cli.MigrationNameFlag)
	migrationName := "disable_user"
	migrationVersion := v.GetString(cli.MigrationVersionFlag)
	migrationEmail := strings.Split(v.GetString(DisableUserEmailFlag), "@")

	err = CheckDisableUserFlags(v)
	if err != nil {
		return err
	}

	emailPrefix := migrationEmail[0]
	emailDomain := migrationEmail[1]

	user := UserTemplate{EmailPrefix: emailPrefix, EmailDomain: emailDomain}

	secureMigrationName := fmt.Sprintf("%s_%s.up.sql", migrationVersion, migrationName)
	t1 := template.Must(template.New("disable_user").Parse(disableUser))
	err = createMigration(tempMigrationPath, secureMigrationName, t1, user)
	if err != nil {
		return err
	}
	localMigrationPath := filepath.Join("local_migrations", secureMigrationName)
	localMigrationFile, err := os.Create(localMigrationPath)
	defer closeFile(localMigrationFile)
	if err != nil {
		return errors.Wrapf(err, "error creating %s", localMigrationPath)
	}
	log.Printf("new migration file created at:  %q\n", localMigrationPath)

	migrationFileName := fmt.Sprintf("%s_%s.up.fizz", migrationVersion, migrationName)
	t2 := template.Must(template.New("migration").Parse(secureMigrationTemplate))
	err = createMigration(migrationsPath, migrationFileName, t2, secureMigrationName)
	if err != nil {
		return err
	}

	err = addMigrationToManifest(migrationManifest, migrationFileName)
	if err != nil {
		return err
	}
	return nil
}
