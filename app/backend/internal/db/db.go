package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"sakuravel/migrations"
)

func New() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatalf("DATABASE_URL is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	for i := 0; i < 10; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("waiting for db... (%d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("db ping: %v", err)
	}

	log.Println("database connected")
	return db
}

// newMigrateInstance creates a new migrate.Migrate instance with the given *sql.DB.
func newMigrateInstance(db *sql.DB) (*migrate.Migrate, error) {
	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create mysql driver: %w", err)
	}

	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to create iofs source driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "mysql", driver)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return m, nil
}

// ResetAndMigrateWithDB drops all tables using golang-migrate and reapplies all migrations.
func ResetAndMigrateWithDB(db *sql.DB) error {
	m, err := newMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Drop(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	mUp, err := newMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := mUp.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("database reset and migrations applied successfully")
	return nil
}

// RunMigrationsWithDB applies all pending database migrations on the given *sql.DB instance.
func RunMigrationsWithDB(db *sql.DB) error {
	m, err := newMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("database migrations applied successfully")
	return nil
}

// RunMigrations applies all pending database migrations using golang-migrate.
// It creates a dedicated temporary connection with multiStatements=true to execute migrations safely.
func RunMigrations() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}
	cfg.MultiStatements = true

	migrationDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("failed to open migration db: %w", err)
	}
	defer func() {
		if closeErr := migrationDB.Close(); closeErr != nil {
			log.Printf("failed to close migration db: %v", closeErr)
		}
	}()

	return RunMigrationsWithDB(migrationDB)
}
