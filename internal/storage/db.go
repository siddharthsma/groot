package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	iauth "groot/internal/auth"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	db *sql.DB
}

func New(ctx context.Context, dsn string) (*DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(checkCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{db: db}, nil
}

func (d *DB) Check(ctx context.Context) error {
	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

type scanner interface {
	Scan(dest ...any) error
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func jsonBytes(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

func parseOptionalUUID(value sql.NullString) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed, err := uuid.Parse(value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

type actorMetadata struct {
	Type  any
	ID    any
	Email any
}

func actorFromContext(ctx context.Context) actorMetadata {
	principal, ok := iauth.PrincipalFromContext(ctx)
	if !ok {
		return actorMetadata{}
	}
	return actorMetadata{
		Type:  nullableString(principal.Actor.Type),
		ID:    nullableString(principal.Actor.ID),
		Email: nullableString(principal.Actor.Email),
	}
}
