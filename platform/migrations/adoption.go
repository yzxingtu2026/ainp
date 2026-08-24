package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// AdoptionPlan is an immutable, reviewable description of one fork takeover.
// It contains identities and ledger facts, never arbitrary SQL.
type AdoptionPlan struct {
	SchemaVersion     int                  `json:"schema_version"`
	Source            AdoptionIdentity     `json:"source"`
	Target            AdoptionIdentity     `json:"target"`
	CommonPrefix      []LedgerEntry        `json:"common_prefix"`
	OldLedger         []LedgerEntry        `json:"old_ledger"`
	Mappings          []AdoptionMapping    `json:"mappings"`
	Bridges           []BridgeStep         `json:"bridges,omitempty"`
	Preconditions     []SchemaPrecondition `json:"preconditions,omitempty"`
	Postconditions    []SchemaPrecondition `json:"postconditions,omitempty"`
	BackupConfirmed   bool                 `json:"backup_confirmed"`
	MaintenanceWindow bool                 `json:"maintenance_window"`
}

type AdoptionIdentity struct {
	Project string `json:"project"`
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
}

type LedgerEntry struct {
	Version  int    `json:"version"`
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
}

type AdoptionMapping struct {
	OldVersion     int    `json:"old_version"`
	OldName        string `json:"old_name"`
	OldChecksum    string `json:"old_checksum"`
	TargetKind     string `json:"target_kind"` // builtin or downstream
	TargetOwner    string `json:"target_owner,omitempty"`
	TargetVersion  int    `json:"target_version"`
	TargetName     string `json:"target_name"`
	TargetChecksum string `json:"target_checksum"`
	Proof          string `json:"proof,omitempty"`
}

type BridgeStep struct {
	ID string `json:"id"`
}
type SchemaPrecondition struct {
	Table  string `json:"table"`
	Column string `json:"column,omitempty"`
}

type AdoptionMode string

const (
	AdoptionDryRun AdoptionMode = "dry-run"
	AdoptionApply  AdoptionMode = "apply"
	AdoptionVerify AdoptionMode = "verify"
)

type AdoptionResult struct {
	PlanDigest         string              `json:"plan_digest"`
	AlreadyComplete    bool                `json:"already_complete"`
	CommonPrefix       int                 `json:"common_prefix"`
	BuiltinToRecord    []LedgerEntry       `json:"builtin_to_record,omitempty"`
	BuiltinToExecute   []LedgerEntry       `json:"builtin_to_execute,omitempty"`
	DownstreamToRecord []NamespacedApplied `json:"downstream_to_record,omitempty"`
	Bridges            []string            `json:"bridges,omitempty"`
}

// BridgeRegistry contains reviewed, code-owned bridge operations.
type BridgeRegistry map[string]func(*gorm.DB) error

type adoptionRecord struct {
	PlanDigest string    `gorm:"primaryKey;size:64"`
	SourceTag  string    `gorm:"size:128;not null"`
	TargetTag  string    `gorm:"size:128;not null"`
	AppliedAt  time.Time `gorm:"not null"`
}

func (adoptionRecord) TableName() string { return "schema_migration_adoptions" }

func (p AdoptionPlan) Digest() (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:]), nil
}

func (p AdoptionPlan) Validate(set Set) error {
	if p.SchemaVersion != 1 {
		return fmt.Errorf("adoption plan schema version must be 1")
	}
	if p.Source.Project == "" || !releaseTagPattern.MatchString(p.Source.Tag) || p.Source.Commit == "" {
		return fmt.Errorf("source identity is incomplete")
	}
	if p.Target.Project == "" || !releaseTagPattern.MatchString(p.Target.Tag) || p.Target.Commit == "" {
		return fmt.Errorf("target must be a released SemVer tag and commit")
	}
	if !p.BackupConfirmed || !p.MaintenanceWindow {
		return fmt.Errorf("backup confirmation and maintenance window are required")
	}
	known, err := Load()
	if err != nil {
		return err
	}
	knownBuiltin := make(map[int]Migration, len(known))
	for _, m := range known {
		knownBuiltin[m.Version] = m
	}
	knownDown := make(map[string]NamespacedMigration, len(set.known))
	for _, m := range set.known {
		knownDown[fmt.Sprintf("%s:%d", m.Owner, m.Version)] = m
	}
	seen := map[int]bool{}
	for _, row := range p.OldLedger {
		if row.Version <= 0 || row.Name == "" || len(row.Checksum) != 64 || seen[row.Version] {
			return fmt.Errorf("old ledger is incomplete or duplicated at version %d", row.Version)
		}
		seen[row.Version] = true
	}
	for _, row := range p.CommonPrefix {
		expected, ok := knownBuiltin[row.Version]
		if !ok || expected.Name != row.Name || expected.Checksum != row.Checksum {
			return fmt.Errorf("common prefix does not match target builtin migration %04d", row.Version)
		}
	}
	mappedOld := map[int]bool{}
	mappedBuiltin := map[int]bool{}
	for _, m := range p.Mappings {
		if !seen[m.OldVersion] || mappedOld[m.OldVersion] || m.OldName == "" || len(m.OldChecksum) != 64 {
			return fmt.Errorf("mapping has no unique exact old ledger identity for %04d", m.OldVersion)
		}
		var old LedgerEntry
		for _, row := range p.OldLedger {
			if row.Version == m.OldVersion {
				old = row
				break
			}
		}
		if old.Name != m.OldName || old.Checksum != m.OldChecksum {
			return fmt.Errorf("mapping old identity mismatch for %04d", m.OldVersion)
		}
		mappedOld[m.OldVersion] = true
		if (m.OldVersion != m.TargetVersion || m.OldName != m.TargetName || m.OldChecksum != m.TargetChecksum) && strings.TrimSpace(m.Proof) == "" {
			return fmt.Errorf("mapping %04d requires explicit equivalence proof", m.OldVersion)
		}
		if m.TargetKind == "builtin" {
			expected, ok := knownBuiltin[m.TargetVersion]
			if !ok || expected.Name != m.TargetName || expected.Checksum != m.TargetChecksum {
				return fmt.Errorf("mapping target builtin %04d is not current", m.TargetVersion)
			}
			if mappedBuiltin[m.TargetVersion] {
				return fmt.Errorf("builtin target %04d is mapped more than once", m.TargetVersion)
			}
			mappedBuiltin[m.TargetVersion] = true
		} else if m.TargetKind == "downstream" {
			expected, ok := knownDown[fmt.Sprintf("%s:%d", m.TargetOwner, m.TargetVersion)]
			if !ok || expected.Name != m.TargetName || expected.Checksum != m.TargetChecksum {
				return fmt.Errorf("mapping target downstream %s:%04d is not registered", m.TargetOwner, m.TargetVersion)
			}
		} else {
			return fmt.Errorf("mapping %04d has unknown target kind", m.OldVersion)
		}
	}
	if len(p.Mappings) != len(p.OldLedger) {
		return fmt.Errorf("every old ledger row requires one explicit mapping")
	}
	if len(p.CommonPrefix) > len(p.OldLedger) {
		return fmt.Errorf("common prefix is longer than old ledger")
	}
	for i, row := range p.CommonPrefix {
		old := p.OldLedger[i]
		if old != row {
			return fmt.Errorf("common prefix is not an exact old ledger prefix at version %04d", row.Version)
		}
	}
	for _, b := range p.Bridges {
		if strings.TrimSpace(b.ID) == "" {
			return fmt.Errorf("bridge id is required")
		}
	}
	return nil
}

func (p AdoptionPlan) Inspect(db *gorm.DB, set Set) (AdoptionResult, error) {
	if err := p.Validate(set); err != nil {
		return AdoptionResult{}, err
	}
	var rows []Applied
	if err := db.Order("version ASC").Find(&rows).Error; err != nil {
		return AdoptionResult{}, fmt.Errorf("read old migration ledger: %w", err)
	}
	if len(rows) != len(p.OldLedger) {
		return AdoptionResult{}, fmt.Errorf("old ledger row count mismatch")
	}
	for i, row := range rows {
		expected := p.OldLedger[i]
		if row.Version != expected.Version || row.Name != expected.Name || row.Checksum != expected.Checksum {
			return AdoptionResult{}, fmt.Errorf("old ledger mismatch at version %04d", row.Version)
		}
	}
	for _, pre := range p.Preconditions {
		if !db.Migrator().HasTable(pre.Table) || (pre.Column != "" && !db.Migrator().HasColumn(pre.Table, pre.Column)) {
			return AdoptionResult{}, fmt.Errorf("schema precondition failed for %s.%s", pre.Table, pre.Column)
		}
	}
	result := AdoptionResult{}
	result.PlanDigest, _ = p.Digest()
	result.CommonPrefix = len(p.CommonPrefix)
	mappedBuiltin := map[int]bool{}
	for _, m := range p.Mappings {
		if m.TargetKind == "builtin" {
			result.BuiltinToRecord = append(result.BuiltinToRecord, LedgerEntry{Version: m.TargetVersion, Name: m.TargetName, Checksum: m.TargetChecksum})
			mappedBuiltin[m.TargetVersion] = true
		} else {
			result.DownstreamToRecord = append(result.DownstreamToRecord, NamespacedApplied{Owner: m.TargetOwner, Version: m.TargetVersion, Name: m.TargetName, Checksum: m.TargetChecksum})
		}
	}
	known, _ := Load()
	for _, m := range known {
		if !mappedBuiltin[m.Version] {
			result.BuiltinToExecute = append(result.BuiltinToExecute, LedgerEntry{Version: m.Version, Name: m.Name, Checksum: m.Checksum})
		}
	}
	sort.Slice(result.BuiltinToRecord, func(i, j int) bool { return result.BuiltinToRecord[i].Version < result.BuiltinToRecord[j].Version })
	sort.Slice(result.DownstreamToRecord, func(i, j int) bool {
		if result.DownstreamToRecord[i].Owner == result.DownstreamToRecord[j].Owner {
			return result.DownstreamToRecord[i].Version < result.DownstreamToRecord[j].Version
		}
		return result.DownstreamToRecord[i].Owner < result.DownstreamToRecord[j].Owner
	})
	for _, b := range p.Bridges {
		result.Bridges = append(result.Bridges, b.ID)
	}
	sort.Strings(result.Bridges)
	return result, nil
}

func (p AdoptionPlan) Apply(db *gorm.DB, set Set, bridges BridgeRegistry, confirmedDigest string) (AdoptionResult, error) {
	digest, err := p.Digest()
	if err != nil {
		return AdoptionResult{}, err
	}
	if confirmedDigest != digest {
		return AdoptionResult{}, fmt.Errorf("plan digest confirmation mismatch")
	}
	if db.Migrator().HasTable(&adoptionRecord{}) {
		var records []adoptionRecord
		if err := db.Find(&records).Error; err != nil {
			return AdoptionResult{}, err
		}
		if len(records) > 0 {
			if len(records) == 1 && records[0].PlanDigest == digest {
				result := AdoptionResult{PlanDigest: digest, AlreadyComplete: true}
				if _, err := set.Inspect(db); err != nil {
					return AdoptionResult{}, err
				}
				return result, nil
			}
			return AdoptionResult{}, fmt.Errorf("database was already adopted by a different plan")
		}
	}
	result, err := p.Inspect(db, set)
	if err != nil {
		return AdoptionResult{}, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", advisoryLockID).Error; err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		for _, b := range p.Bridges {
			fn, ok := bridges[b.ID]
			if !ok {
				return fmt.Errorf("bridge %q is not registered", b.ID)
			}
			if err := fn(tx); err != nil {
				return fmt.Errorf("bridge %q failed: %w", b.ID, err)
			}
		}
		for _, post := range p.Postconditions {
			if !tx.Migrator().HasTable(post.Table) || (post.Column != "" && !tx.Migrator().HasColumn(post.Table, post.Column)) {
				return fmt.Errorf("schema postcondition failed for %s.%s", post.Table, post.Column)
			}
		}
		if err := tx.Exec("TRUNCATE schema_migrations").Error; err != nil {
			return err
		}
		mapped := map[int]LedgerEntry{}
		for _, m := range result.BuiltinToRecord {
			mapped[m.Version] = m
		}
		known, _ := Load()
		for _, migration := range known {
			if _, ok := mapped[migration.Version]; !ok {
				if err := tx.Exec(migration.SQL).Error; err != nil {
					return fmt.Errorf("execute %s: %w", migration.Name, err)
				}
			}
			if err := tx.Create(&Applied{Version: migration.Version, Name: migration.Name, Checksum: migration.Checksum, AppliedAt: time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		if err := tx.AutoMigrate(&NamespacedApplied{}); err != nil {
			return err
		}
		for _, m := range result.DownstreamToRecord {
			m.AppliedAt = time.Now().UTC()
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
		}
		if err := tx.AutoMigrate(&adoptionRecord{}); err != nil {
			return err
		}
		if err := tx.Create(&adoptionRecord{PlanDigest: result.PlanDigest, SourceTag: p.Source.Tag, TargetTag: p.Target.Tag, AppliedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return AdoptionResult{}, err
	}
	if _, err := set.Inspect(db); err != nil {
		return AdoptionResult{}, fmt.Errorf("post-adoption inspection failed: %w", err)
	}
	return result, nil
}
