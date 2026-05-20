package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestVisitRepositoryDeleteVisitsBefore(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewVisitRepository(db)
	before := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM visit_log WHERE visited_at < ? LIMIT ?")).
		WithArgs(before, 500).
		WillReturnResult(sqlmock.NewResult(0, 3))

	deleted, err := repo.DeleteVisitsBefore(context.Background(), before, 500)
	if err != nil {
		t.Fatalf("DeleteVisitsBefore returned error: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestVisitRepositoryDeleteDailyStatsBefore(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewVisitRepository(db)
	before := time.Date(2026, 1, 20, 13, 30, 0, 0, time.UTC)
	cutoffDate := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM link_stat_daily WHERE stat_date < ? LIMIT ?")).
		WithArgs(cutoffDate, 1000).
		WillReturnResult(sqlmock.NewResult(0, 2))

	deleted, err := repo.DeleteDailyStatsBefore(context.Background(), before, 1000)
	if err != nil {
		t.Fatalf("DeleteDailyStatsBefore returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
