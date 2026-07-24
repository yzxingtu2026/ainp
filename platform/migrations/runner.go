package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
)

//go:embed *.sql
var files embed.FS

var migrationNamePattern = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

const advisoryLockID int64 = 683446821735171833

type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

type Applied struct {
	Version   int       `gorm:"primaryKey"`
	Name      string    `gorm:"size:255;not null;uniqueIndex"`
	Checksum  string    `gorm:"size:64;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

func (Applied) TableName() string { return "schema_migrations" }

type Status struct {
	Initialized bool
	Applied     []Applied
	Pending     []Migration
}

func Load() ([]Migration, error) {
	return loadFS(files)
}

func loadFS(source fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, err
	}
	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !migrationNamePattern.MatchString(entry.Name()) {
			continue
		}
		version, _ := strconv.Atoi(migrationNamePattern.FindStringSubmatch(entry.Name())[1])
		if previous := seen[version]; previous != "" {
			return nil, fmt.Errorf("duplicate migration version %04d: %s and %s", version, previous, entry.Name())
		}
		body, readErr := fs.ReadFile(source, entry.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), readErr)
		}
		digest := sha256.Sum256(body)
		seen[version] = entry.Name()
		migrations = append(migrations, Migration{Version: version, Name: entry.Name(), SQL: string(body), Checksum: hex.EncodeToString(digest[:])})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no migrations found")
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version != migrations[i-1].Version+1 {
			return nil, fmt.Errorf("migration sequence gap between %04d and %04d", migrations[i-1].Version, migrations[i].Version)
		}
	}
	return migrations, nil
}

func Inspect(db *gorm.DB) (Status, error) {
	known, err := Load()
	if err != nil {
		return Status{}, err
	}
	initialized := db.Migrator().HasTable("users")
	if !initialized {
		return Status{Initialized: false, Pending: known}, nil
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
version integer PRIMARY KEY,
name varchar(255) NOT NULL UNIQUE,
checksum varchar(64) NOT NULL,
applied_at timestamptz NOT NULL
)`).Error; err != nil {
		return Status{}, fmt.Errorf("create migration state: %w", err)
	}
	var applied []Applied
	if err := db.Order("version ASC").Find(&applied).Error; err != nil {
		return Status{}, fmt.Errorf("read migration state: %w", err)
	}
	if err := validateApplied(known, applied); err != nil {
		return Status{}, err
	}
	return Status{Initialized: true, Applied: applied, Pending: known[len(applied):]}, nil
}

func validateApplied(known []Migration, applied []Applied) error {
	if len(applied) > len(known) {
		return fmt.Errorf("database contains unknown migration versions")
	}
	for i, row := range applied {
		expected := known[i]
		if row.Version != expected.Version || row.Name != expected.Name {
			return fmt.Errorf("migration history is out of order at version %04d", row.Version)
		}
		if row.Checksum != expected.Checksum {
			return fmt.Errorf("migration checksum mismatch for %s", row.Name)
		}
	}
	return nil
}

func Up(db *gorm.DB) (Status, error) {
	status, err := Inspect(db)
	if err != nil || !status.Initialized || len(status.Pending) == 0 {
		return status, err
	}
	if len(status.Applied) == 0 {
		return Status{}, fmt.Errorf("migration history is empty; verify the current schema and run baseline once")
	}
	if err := db.Exec("SELECT pg_advisory_lock(?)", advisoryLockID).Error; err != nil {
		return Status{}, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer db.Exec("SELECT pg_advisory_unlock(?)", advisoryLockID)

	status, err = Inspect(db)
	if err != nil {
		return Status{}, err
	}
	for _, migration := range status.Pending {
		err = db.Transaction(func(tx *gorm.DB) error {
			if execErr := tx.Exec(migration.SQL).Error; execErr != nil {
				return fmt.Errorf("execute %s: %w", migration.Name, execErr)
			}
			row := Applied{Version: migration.Version, Name: migration.Name, Checksum: migration.Checksum, AppliedAt: time.Now().UTC()}
			if createErr := tx.Create(&row).Error; createErr != nil {
				return fmt.Errorf("record %s: %w", migration.Name, createErr)
			}
			return nil
		})
		if err != nil {
			return Status{}, err
		}
	}
	return Inspect(db)
}

func Baseline(db *gorm.DB) (Status, error) {
	status, err := Inspect(db)
	if err != nil || !status.Initialized {
		return status, err
	}
	if len(status.Applied) != 0 {
		return Status{}, fmt.Errorf("migration baseline requires empty history")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, migration := range status.Pending {
			row := Applied{Version: migration.Version, Name: migration.Name, Checksum: migration.Checksum, AppliedAt: time.Now().UTC()}
			if createErr := tx.Create(&row).Error; createErr != nil {
				return fmt.Errorf("baseline %s: %w", migration.Name, createErr)
			}
		}
		return nil
	}); err != nil {
		return Status{}, err
	}
	return Inspect(db)
}
