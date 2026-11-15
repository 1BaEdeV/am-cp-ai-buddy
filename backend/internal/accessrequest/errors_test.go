package accessrequest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"no error", nil, false},
		{"gorm duplicate", gorm.ErrDuplicatedKey, true},
		{"wrapped gorm duplicate", fmt.Errorf("create request: %w", gorm.ErrDuplicatedKey), true},
		{"postgres duplicate", &pgconn.PgError{Code: "23505"}, true},
		{"wrapped postgres duplicate", fmt.Errorf("create request: %w", &pgconn.PgError{Code: "23505"}), true},
		{"joined duplicate", errors.Join(errors.New("operation failed"), &pgconn.PgError{Code: "23505"}), true},
		{"foreign key violation", &pgconn.PgError{Code: "23503"}, false},
		{"check violation", &pgconn.PgError{Code: "23514"}, false},
		{"not found", gorm.ErrRecordNotFound, false},
		{"error text is not a sql state", errors.New("23505 duplicate key"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isUniqueViolation(tt.err))
		})
	}
}
