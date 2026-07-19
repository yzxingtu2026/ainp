package tenant

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository[T any] struct{ db *gorm.DB }

type Result struct {
	RowsAffected int64
	Error        error
}

func (r *Repository[T]) query() *gorm.DB {
	return r.db.Session(&gorm.Session{})
}

func cleanRepositoryDB(db *gorm.DB) *gorm.DB {
	q := db.Session(&gorm.Session{NewDB: true})
	if mode, ok := db.Get(accessModeKey); ok {
		q = q.Set(accessModeKey, mode)
	}
	return q
}

func (r *Repository[T]) clone(db *gorm.DB) *Repository[T] { return &Repository[T]{db: db} }
func (r *Repository[T]) Where(query any, args ...any) *Repository[T] {
	return r.clone(r.query().Where(query, args...))
}
func (r *Repository[T]) Order(value any) *Repository[T] { return r.clone(r.query().Order(value)) }
func (r *Repository[T]) Clauses(conds ...clause.Expression) *Repository[T] {
	return r.clone(r.query().Clauses(conds...))
}

func NewTenantRepository[T any](db TenantDatabase) (*Repository[T], error) {
	if db.db == nil {
		return nil, ErrTenantDBRequired
	}
	return &Repository[T]{db: cleanRepositoryDB(db.db)}, nil
}
func NewSystemRepository[T any](db SystemDatabase) (*Repository[T], error) {
	if db.db == nil {
		return nil, ErrSystemDBRequired
	}
	return &Repository[T]{db: cleanRepositoryDB(db.db)}, nil
}
func (r *Repository[T]) First(where any, args ...any) (T, error) {
	var v T
	err := r.query().Where(where, args...).First(&v).Error
	return v, err
}
func (r *Repository[T]) GetByID(id any) (T, error) { return r.First("id = ?", id) }
func (r *Repository[T]) Find(where any, args ...any) ([]T, error) {
	var v []T
	err := r.query().Where(where, args...).Find(&v).Error
	return v, err
}
func (r *Repository[T]) List(offset, limit int) ([]T, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var v []T
	err := r.query().Offset(offset).Limit(limit).Find(&v).Error
	return v, err
}
func (r *Repository[T]) Count() (int64, error) {
	var n int64
	err := r.query().Model(new(T)).Count(&n).Error
	return n, err
}
func (r *Repository[T]) Exists(where any, args ...any) (bool, error) {
	var n int64
	err := r.query().Model(new(T)).Where(where, args...).Limit(1).Count(&n).Error
	return n > 0, err
}
func (r *Repository[T]) Create(v *T) error { return r.query().Create(v).Error }
func (r *Repository[T]) CreateBatch(v []T, size int) error {
	if size <= 0 {
		size = 100
	}
	return r.query().CreateInBatches(v, size).Error
}
func (r *Repository[T]) UpdateRevision(id any, revision uint64, values map[string]any) error {
	res := r.query().Model(new(T)).Where("id = ? AND revision = ?", id, revision).Updates(values)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *Repository[T]) Updates(values map[string]any) Result {
	res := r.query().Model(new(T)).Updates(values)
	return Result{RowsAffected: res.RowsAffected, Error: res.Error}
}
func (r *Repository[T]) DeleteWhere(where any, args ...any) Result {
	res := r.query().Where(where, args...).Delete(new(T))
	return Result{RowsAffected: res.RowsAffected, Error: res.Error}
}
func (r *Repository[T]) Delete(v *T) error { return r.query().Delete(v).Error }
func IsNotFound(err error) bool            { return errors.Is(err, gorm.ErrRecordNotFound) }
