package tenant

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InstallCallbacks(db *gorm.DB, registry *Registry) error {
	if db == nil || registry == nil {
		return fmt.Errorf("tenant callbacks require database and registry")
	}
	query := func(tx *gorm.DB) { guardQuery(tx, registry) }
	write := func(tx *gorm.DB) { guardWrite(tx, registry) }
	for _, item := range []struct {
		name     string
		register func(string, func(*gorm.DB)) error
	}{
		{"query", db.Callback().Query().Before("gorm:query").Register},
		{"row", db.Callback().Row().Before("gorm:row").Register},
		{"create", db.Callback().Create().Before("gorm:create").Register},
		{"update", db.Callback().Update().Before("gorm:update").Register},
		{"delete", db.Callback().Delete().Before("gorm:delete").Register},
	} {
		fn := query
		if item.name == "create" || item.name == "update" {
			fn = write
		}
		if err := item.register("ainp:tenant:"+item.name, fn); err != nil {
			return err
		}
	}
	return nil
}

func registered(tx *gorm.DB, r *Registry) (ModelInfo, bool) {
	if tx.Statement == nil {
		return ModelInfo{}, false
	}
	if tx.Statement.Model != nil {
		if i, ok := r.Lookup(tx.Statement.Model); ok {
			return i, true
		}
	}
	if tx.Statement.Dest != nil {
		return r.Lookup(tx.Statement.Dest)
	}
	return ModelInfo{}, false
}
func guardQuery(tx *gorm.DB, r *Registry) {
	info, ok := registered(tx, r)
	if !ok || info.Kind == GlobalModel {
		return
	}
	mode, tenantID, err := modeAndTenant(tx)
	if err != nil {
		_ = tx.AddError(err)
		return
	}
	if mode == modeSystem {
		if !info.SystemAccess {
			_ = tx.AddError(ErrSystemDBRequired)
		}
		return
	}
	column := clause.Column{Table: clause.CurrentTable, Name: info.TenantColumn}
	tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{Column: column, Value: tenantID}}})
}
func guardWrite(tx *gorm.DB, r *Registry) {
	info, ok := registered(tx, r)
	if !ok || info.Kind == GlobalModel {
		return
	}
	mode, tenantID, err := modeAndTenant(tx)
	if err != nil {
		_ = tx.AddError(err)
		return
	}
	if mode == modeSystem {
		if !info.SystemAccess {
			_ = tx.AddError(ErrSystemDBRequired)
		}
		return
	}
	if tx.Statement.Schema != nil {
		field := tx.Statement.Schema.LookUpField("TenantID")
		if field != nil {
			rv := tx.Statement.ReflectValue
			for rv.Kind() == reflect.Pointer {
				rv = rv.Elem()
			}
			set := func(v reflect.Value) {
				current, _ := field.ValueOf(tx.Statement.Context, v)
				s, _ := current.(string)
				if strings.TrimSpace(s) != "" && s != tenantID {
					_ = tx.AddError(ErrTenantConflict)
					return
				}
				if info.InjectOnCreate && s == "" {
					_ = field.Set(tx.Statement.Context, v, tenantID)
				}
			}
			if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
				for i := 0; i < rv.Len(); i++ {
					set(rv.Index(i))
				}
			} else if rv.IsValid() {
				set(rv)
			}
		}
	}
	if tx.Error == nil {
		guardQuery(tx, r)
	}
}
