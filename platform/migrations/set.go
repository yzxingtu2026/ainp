package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ownerPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

// Source is one downstream module's immutable forward-only migration stream.
// Files must use the same NNNN_name.sql convention as built-in migrations.
type Source struct {
	Owner string
	FS    fs.FS
}

type NamespacedMigration struct {
	Owner    string
	Version  int
	Name     string
	SQL      string
	Checksum string
}

type NamespacedApplied struct {
	Owner     string    `gorm:"primaryKey;size:128"`
	Version   int       `gorm:"primaryKey"`
	Name      string    `gorm:"size:255;not null"`
	Checksum  string    `gorm:"size:64;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

func (NamespacedApplied) TableName() string { return "schema_module_migrations" }

type SetStatus struct {
	Initialized       bool
	Builtin           Status
	AppliedDownstream []NamespacedApplied
	PendingDownstream []NamespacedMigration
}

// Set combines the immutable built-in history with sorted downstream sources.
type Set struct{ known []NamespacedMigration }

func NewSet(sources ...Source) (Set, error) {
	seenOwners := make(map[string]struct{}, len(sources))
	known := make([]NamespacedMigration, 0)
	for _, source := range sources {
		owner := strings.TrimSpace(source.Owner)
		if !ownerPattern.MatchString(owner) || owner == "ainp" {
			return Set{}, fmt.Errorf("invalid migration owner %q", owner)
		}
		if source.FS == nil {
			return Set{}, fmt.Errorf("migration source %q requires filesystem", owner)
		}
		if _, exists := seenOwners[owner]; exists {
			return Set{}, fmt.Errorf("duplicate migration owner %q", owner)
		}
		seenOwners[owner] = struct{}{}
		loaded, err := loadNamespaced(source)
		if err != nil {
			return Set{}, err
		}
		known = append(known, loaded...)
	}
	sort.Slice(known, func(i, j int) bool {
		if known[i].Owner == known[j].Owner {
			return known[i].Version < known[j].Version
		}
		return known[i].Owner < known[j].Owner
	})
	return Set{known: known}, nil
}

func loadNamespaced(source Source) ([]NamespacedMigration, error) {
	entries, err := fs.ReadDir(source.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("migration owner %q: %w", source.Owner, err)
	}
	loaded := make([]NamespacedMigration, 0, len(entries))
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !migrationNamePattern.MatchString(entry.Name()) {
			continue
		}
		version, _ := strconv.Atoi(migrationNamePattern.FindStringSubmatch(entry.Name())[1])
		if previous := seen[version]; previous != "" {
			return nil, fmt.Errorf("migration owner %q duplicate version %04d: %s and %s", source.Owner, version, previous, entry.Name())
		}
		body, readErr := fs.ReadFile(source.FS, entry.Name())
		if readErr != nil {
			return nil, fmt.Errorf("migration owner %q read %s: %w", source.Owner, entry.Name(), readErr)
		}
		digest := sha256.Sum256(body)
		seen[version] = entry.Name()
		loaded = append(loaded, NamespacedMigration{Owner: source.Owner, Version: version, Name: entry.Name(), SQL: string(body), Checksum: hex.EncodeToString(digest[:])})
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("migration owner %q has no migrations", source.Owner)
	}
	sort.Slice(loaded, func(i, j int) bool { return loaded[i].Version < loaded[j].Version })
	for i := 1; i < len(loaded); i++ {
		if loaded[i].Version != loaded[i-1].Version+1 {
			return nil, fmt.Errorf("migration owner %q sequence gap between %04d and %04d", source.Owner, loaded[i-1].Version, loaded[i].Version)
		}
	}
	return loaded, nil
}

func (s Set) Inspect(db *gorm.DB) (SetStatus, error) {
	builtin, err := Inspect(db)
	if err != nil {
		return SetStatus{}, err
	}
	status := SetStatus{Initialized: builtin.Initialized, Builtin: builtin, PendingDownstream: append([]NamespacedMigration(nil), s.known...)}
	if !builtin.Initialized {
		return status, nil
	}
	if err := db.AutoMigrate(&NamespacedApplied{}); err != nil {
		return SetStatus{}, fmt.Errorf("create downstream migration state: %w", err)
	}
	if err := db.Order("owner ASC, version ASC").Find(&status.AppliedDownstream).Error; err != nil {
		return SetStatus{}, fmt.Errorf("read downstream migration state: %w", err)
	}
	pending, err := validateNamespacedApplied(s.known, status.AppliedDownstream)
	if err != nil {
		return SetStatus{}, err
	}
	status.PendingDownstream = pending
	return status, nil
}

func validateNamespacedApplied(known []NamespacedMigration, applied []NamespacedApplied) ([]NamespacedMigration, error) {
	knownByID := make(map[string]NamespacedMigration, len(known))
	for _, migration := range known {
		knownByID[fmt.Sprintf("%s:%04d", migration.Owner, migration.Version)] = migration
	}
	appliedIDs := make(map[string]struct{}, len(applied))
	for _, row := range applied {
		id := fmt.Sprintf("%s:%04d", row.Owner, row.Version)
		expected, exists := knownByID[id]
		if !exists {
			return nil, fmt.Errorf("database contains unknown downstream migration %s", id)
		}
		if row.Name != expected.Name {
			return nil, fmt.Errorf("downstream migration name mismatch for %s", id)
		}
		if row.Checksum != expected.Checksum {
			return nil, fmt.Errorf("downstream migration checksum mismatch for %s (%s)", id, row.Name)
		}
		appliedIDs[id] = struct{}{}
	}
	pending := make([]NamespacedMigration, 0, len(known)-len(applied))
	missingByOwner := make(map[string]bool)
	for _, migration := range known {
		id := fmt.Sprintf("%s:%04d", migration.Owner, migration.Version)
		_, exists := appliedIDs[id]
		if exists && missingByOwner[migration.Owner] {
			return nil, fmt.Errorf("downstream migration history is out of order at %s", id)
		}
		if !exists {
			missingByOwner[migration.Owner] = true
			pending = append(pending, migration)
		}
	}
	return pending, nil
}

func (s Set) Up(db *gorm.DB) (SetStatus, error) {
	if _, err := Up(db); err != nil {
		return SetStatus{}, err
	}
	status, err := s.Inspect(db)
	if err != nil || !status.Initialized || len(status.PendingDownstream) == 0 {
		return status, err
	}
	if err := s.applyDownstream(db, status.PendingDownstream, true); err != nil {
		return SetStatus{}, err
	}
	return s.Inspect(db)
}

func (s Set) Baseline(db *gorm.DB) (SetStatus, error) {
	if _, err := Baseline(db); err != nil {
		return SetStatus{}, err
	}
	status, err := s.Inspect(db)
	if err != nil || !status.Initialized || len(status.PendingDownstream) == 0 {
		return status, err
	}
	if len(status.AppliedDownstream) != 0 {
		return SetStatus{}, fmt.Errorf("downstream migration baseline requires empty history")
	}
	if err := s.applyDownstream(db, status.PendingDownstream, false); err != nil {
		return SetStatus{}, err
	}
	return s.Inspect(db)
}

// BaselineThrough records only the built-in history through maxVersion. It
// deliberately leaves all downstream migrations pending because their schema
// state cannot be inferred from the AINP version.
func (s Set) BaselineThrough(db *gorm.DB, maxVersion int) (SetStatus, error) {
	if _, err := BaselineThrough(db, maxVersion); err != nil {
		return SetStatus{}, err
	}
	return s.Inspect(db)
}

func (s Set) applyDownstream(db *gorm.DB, pending []NamespacedMigration, execute bool) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", advisoryLockID).Error; err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		for _, migration := range pending {
			if execute {
				if err := tx.Exec(migration.SQL).Error; err != nil {
					return fmt.Errorf("execute %s:%04d (%s): %w", migration.Owner, migration.Version, migration.Name, err)
				}
			}
			row := NamespacedApplied{Owner: migration.Owner, Version: migration.Version, Name: migration.Name, Checksum: migration.Checksum, AppliedAt: time.Now().UTC()}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("record %s:%04d (%s): %w", migration.Owner, migration.Version, migration.Name, err)
			}
		}
		return nil
	})
}
