package cli

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/gobuffalo/pop"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	// DbDebugFlag is the DB Debug flag
	DbDebugFlag string = "db-debug"
	// DbEnvFlag is the DB environment flag
	DbEnvFlag string = "db-env"
	// DbNameFlag is the DB name flag
	DbNameFlag string = "db-name"
	// DbHostFlag is the DB host flag
	DbHostFlag string = "db-host"
	// DbPortFlag is the DB port flag
	DbPortFlag string = "db-port"
	// DbUserFlag is the DB user flag
	DbUserFlag string = "db-user"
	// DbPasswordFlag is the DB password flag
	DbPasswordFlag string = "db-password"
	// DbPoolFlag is the DB pool flag
	DbPoolFlag string = "db-pool"
	// DbIdlePoolFlag is the DB idle pool flag
	DbIdlePoolFlag string = "db-idle-pool"
	// DbSSLModeFlag is the DB SSL Mode flag
	DbSSLModeFlag string = "db-ssl-mode"
	// DbSSLRootCertFlag is the DB SSL Root Cert flag
	DbSSLRootCertFlag string = "db-ssl-root-cert"

	// DbEnvContainer is the Container DB Env name
	DbEnvContainer string = "container"
	// DbEnvTest is the Test DB Env name
	DbEnvTest string = "test"
	// DbEnvDevelopment is the Development DB Env name
	DbEnvDevelopment string = "development"

	// The name of the test database
	DbNameTest string = "test_db"

	// SSLModeDisable is the disable SSL Mode
	SSLModeDisable string = "disable"
	// SSLModeAllow is the allow SSL Mode
	SSLModeAllow string = "allow"
	// SSLModePrefer is the prefer SSL Mode
	SSLModePrefer string = "prefer"
	// SSLModeRequire is the require SSL Mode
	SSLModeRequire string = "require"
	// SSLModeVerifyCA is the verify-ca SSL Mode
	SSLModeVerifyCA string = "verify-ca"
	// SSLModeVerifyFull is the verify-full SSL Mode
	SSLModeVerifyFull string = "verify-full"

	// DbPoolDefault is the default db pool connections
	DbPoolDefault = 50
	// DbIdlePoolDefault is the default db idle pool connections
	DbIdlePoolDefault = 2
	// DbPoolMax is the upper limit the db pool can use for connections which constrains the user input
	DbPoolMax int = 50
)

// The dependency https://github.com/lib/pq only supports a limited subset of SSL Modes and returns the error:
// pq: unsupported sslmode \"prefer\"; only \"require\" (default), \"verify-full\", \"verify-ca\", and \"disable\" supported
// - https://www.postgresql.org/docs/10/libpq-ssl.html
var allSSLModes = []string{
	SSLModeDisable,
	// SSLModeAllow,
	// SSLModePrefer,
	SSLModeRequire,
	SSLModeVerifyCA,
	SSLModeVerifyFull,
}

var containerSSLModes = []string{
	SSLModeRequire,
	SSLModeVerifyCA,
	SSLModeVerifyFull,
}

var allDbEnvs = []string{
	DbEnvContainer,
	DbEnvTest,
	DbEnvDevelopment,
}

type errInvalidDbPool struct {
	DbPool int
}

func (e *errInvalidDbPool) Error() string {
	return fmt.Sprintf("invalid db pool of %d. Pool must be greater than 0 and less than or equal to %d", e.DbPool, DbPoolMax)
}

type errInvalidDbIdlePool struct {
	DbPool     int
	DbIdlePool int
}

func (e *errInvalidDbIdlePool) Error() string {
	return fmt.Sprintf("invalid db idle pool of %d. Pool must be greater than 0 and less than or equal to %d", e.DbIdlePool, e.DbPool)
}

type errInvalidDbEnv struct {
	Value  string
	DbEnvs []string
}

func (e *errInvalidDbEnv) Error() string {
	return fmt.Sprintf("invalid db env %s, must be one of: ", e.Value) + strings.Join(e.DbEnvs, ", ")
}

type errInvalidSSLMode struct {
	Mode  string
	Modes []string
}

func (e *errInvalidSSLMode) Error() string {
	return fmt.Sprintf("invalid ssl mode %s, must be one of: "+strings.Join(e.Modes, ", "), e.Mode)
}

// InitDatabaseFlags initializes DB command line flags
func InitDatabaseFlags(flag *pflag.FlagSet) {
	flag.String(DbEnvFlag, DbEnvDevelopment, "database environment: "+strings.Join(allDbEnvs, ", "))
	flag.String(DbNameFlag, "dev_db", "Database Name")
	flag.String(DbHostFlag, "localhost", "Database Hostname")
	flag.Int(DbPortFlag, 5432, "Database Port")
	flag.String(DbUserFlag, "postgres", "Database Username")
	flag.String(DbPasswordFlag, "", "Database Password")
	flag.Int(DbPoolFlag, DbPoolDefault, "Database Pool or max DB connections")
	flag.Int(DbIdlePoolFlag, DbIdlePoolDefault, "Database Idle Pool or max DB idle connections")
	flag.String(DbSSLModeFlag, SSLModeDisable, "Database SSL Mode: "+strings.Join(allSSLModes, ", "))
	flag.String(DbSSLRootCertFlag, "", "Path to the database root certificate file used for database connections")
	flag.Bool(DbDebugFlag, false, "Set Pop to debug mode")
}

// CheckDatabase validates DB command line flags
func CheckDatabase(v *viper.Viper, logger Logger) error {

	if err := ValidateHost(v, DbHostFlag); err != nil {
		return err
	}

	if err := ValidatePort(v, DbPortFlag); err != nil {
		return err
	}

	dbPool := v.GetInt(DbPoolFlag)
	dbIdlePool := v.GetInt(DbIdlePoolFlag)
	if dbPool < 1 || dbPool > DbPoolMax {
		return &errInvalidDbPool{DbPool: dbPool}
	}

	if dbIdlePool > dbPool {
		return &errInvalidDbIdlePool{DbPool: dbPool, DbIdlePool: dbIdlePool}
	}

	dbEnv := v.GetString(DbEnvFlag)
	if !stringSliceContains(allDbEnvs, dbEnv) {
		return &errInvalidDbEnv{Value: dbEnv, DbEnvs: allDbEnvs}
	}

	sslMode := v.GetString(DbSSLModeFlag)
	if len(sslMode) == 0 || !stringSliceContains(allSSLModes, sslMode) {
		return &errInvalidSSLMode{Mode: sslMode, Modes: allSSLModes}
	}
	if dbEnv == DbEnvContainer && !stringSliceContains(containerSSLModes, sslMode) {
		return errors.Wrap(&errInvalidSSLMode{Mode: sslMode, Modes: containerSSLModes}, "container db env requires SSL connection to the database")
	} else if dbEnv != DbEnvContainer && !stringSliceContains(allSSLModes, sslMode) {
		return &errInvalidSSLMode{Mode: sslMode, Modes: allSSLModes}
	}

	if filename := v.GetString(DbSSLRootCertFlag); len(filename) > 0 {
		b, err := ioutil.ReadFile(filename) // #nosec
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error reading %s at %q", DbSSLRootCertFlag, filename))
		}
		tlsCerts := ParseCertificates(string(b))
		logger.Debug(fmt.Sprintf("certificate chain from %s parsed", DbSSLRootCertFlag), zap.Any("count", len(tlsCerts)))
	}

	return nil
}

// InitDatabase initializes a Pop connection from command line flags
func InitDatabase(v *viper.Viper, logger Logger) (*pop.Connection, error) {

	dbEnv := v.GetString(DbEnvFlag)
	dbName := v.GetString(DbNameFlag)
	dbHost := v.GetString(DbHostFlag)
	dbPort := strconv.Itoa(v.GetInt(DbPortFlag))
	dbUser := v.GetString(DbUserFlag)
	dbPassword := v.GetString(DbPasswordFlag)
	dbPool := v.GetInt(DbPoolFlag)
	dbIdlePool := v.GetInt(DbIdlePoolFlag)

	// Modify DB options by environment
	dbOptions := map[string]string{
		"sslmode": v.GetString(DbSSLModeFlag),
	}

	if dbEnv == DbEnvTest {
		// Leave the test database name hardcoded, since we run tests in the same
		// environment as development, and it's extra confusing to have to swap environment
		// variables before running tests.
		dbName = DbNameTest
	}

	if str := v.GetString(DbSSLRootCertFlag); len(str) > 0 {
		dbOptions["sslrootcert"] = str
	}

	// Construct a safe URL and log it
	s := "postgres://%s:%s@%s:%s/%s?sslmode=%s"
	dbURL := fmt.Sprintf(s, dbUser, "*****", dbHost, dbPort, dbName, dbOptions["sslmode"])
	logger.Info("Connecting to the database", zap.String("url", dbURL), zap.String(DbSSLRootCertFlag, v.GetString(DbSSLRootCertFlag)))

	// Configure DB connection details
	dbConnectionDetails := pop.ConnectionDetails{
		Dialect:  "postgres",
		Database: dbName,
		Host:     dbHost,
		Port:     dbPort,
		User:     dbUser,
		Password: dbPassword,
		Options:  dbOptions,
		Pool:     dbPool,
		IdlePool: dbIdlePool,
	}
	err := dbConnectionDetails.Finalize()
	if err != nil {
		logger.Error("Failed to finalize DB connection details", zap.Error(err))
		return nil, err
	}

	// Set up the connection
	connection, err := pop.NewConnection(&dbConnectionDetails)
	if err != nil {
		logger.Error("Failed create DB connection", zap.Error(err))
		return nil, err
	}

	// Open the connection
	err = connection.Open()
	if err != nil {
		logger.Error("Failed to open DB connection", zap.Error(err))
		return nil, err
	}

	// Check the connection
	db, err := sqlx.Open(connection.Dialect.Details().Dialect, connection.Dialect.URL())
	err = db.Ping()
	if err != nil {
		logger.Warn("Failed to ping DB connection", zap.Error(err))
		return connection, err
	}

	// Return the open connection
	return connection, nil
}
