package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRotateTokenDeletesSameNameThenInserts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_tokens`).
		WithArgs("user-1", "local-cli").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO api_tokens`).
		WithArgs("user-1", sqlmock.AnyArg(), "local-cli").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := rotateToken(context.Background(), db, "user-1", "local-cli", "hash"); err != nil {
		t.Fatalf("rotateToken() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRotateTokenLeavesOtherNamesUntouched(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_tokens WHERE user_id = \$1 AND name = \$2`).
		WithArgs("user-1", "local-cli").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO api_tokens`).
		WithArgs("user-1", sqlmock.AnyArg(), "local-cli").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := rotateToken(context.Background(), db, "user-1", "local-cli", "hash"); err != nil {
		t.Fatalf("rotateToken() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRotateTokenRollsBackOnDeleteError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_tokens`).
		WithArgs("user-1", "local-cli").
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err = rotateToken(context.Background(), db, "user-1", "local-cli", "hash")
	if err == nil {
		t.Fatal("rotateToken() error = nil, want error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
