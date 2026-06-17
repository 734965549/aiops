package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation 判断 err 是否为 PostgreSQL 唯一约束冲突（23505）。
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// MapUniqueViolation 将 PG 23505 映射为 target；其它错误原样返回。
func MapUniqueViolation(err error, target error) error {
	if err == nil {
		return nil
	}
	if IsUniqueViolation(err) {
		return target
	}
	return err
}
