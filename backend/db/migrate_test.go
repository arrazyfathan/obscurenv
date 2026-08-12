package db

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSchemaIncludesProjectListDependencies(t *testing.T) {
	for _, table := range requiredTables {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("schema does not create %s", table)
		}
	}
}

func TestVerifyRequiredTablesFailsWhenProjectListDependencyIsMissing(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("projects").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("project_members").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	err = verifyRequiredTables(database)
	if err == nil || !strings.Contains(err.Error(), "project_members") {
		t.Fatalf("verifyRequiredTables error = %v, want missing project_members", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}
