package tenant

import (
	"errors"

	"gorm.io/gorm"
)

type Repository[T any] struct{ db *gorm.DB }

func (r *Repository[T]) query() *gorm.DB {
	q := r.db.Session(&gorm.Session{NewDB: true})
	if mode, ok := r.db.Get(accessModeKey); ok {
		q = q.Set(accessModeKey, mode)
	}
	return q
}

func NewTenantRepository[T any](db TenantDatabase) (*Repository[T], error) {
	if db.db == nil {
		return nil, ErrTenantDBRequired
	}
	return &Repository[T]{db: db.db}, nil
}
func NewSystemRepository[T any](db SystemDatabase) (*Repository[T], error) {
	if db.db == nil {
		return nil, ErrSystemDBRequired
	}
	return &Repository[T]{db: db.db}, nil
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
func (r *Repository[T]) Delete(v *T) error { return r.query().Delete(v).Error }
func IsNotFound(err error) bool            { return errors.Is(err, gorm.ErrRecordNotFound) }
