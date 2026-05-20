package mysql

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jd/flashlink/internal/domain/link"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestShortLinkRepositoryCreate(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewShortLinkRepository(db)
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	item := link.ShortLink{
		ID:        31,
		Code:      link.NewShortCode(31),
		LongURL:   "https://example.com/campaign/31",
		Domain:    "sho.rt",
		Status:    link.ShortLinkStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `short_link_15` (`id`,`code`,`long_url`,`domain`,`expire_at`,`status`,`created_at`,`updated_at`) VALUES (?,?,?,?,?,?,?,?)")).
		WithArgs(item.ID, item.Code, item.LongURL, item.Domain, nil, uint8(item.Status), item.CreatedAt, item.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.Create(context.Background(), item); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestShortLinkRepositoryCreateRejectsInvalidCode(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewShortLinkRepository(db)
	err := repo.Create(context.Background(), link.ShortLink{
		ID:      1,
		Code:    "bad-code",
		LongURL: "https://example.com",
		Status:  link.ShortLinkStatusActive,
	})
	if !errors.Is(err, link.ErrInvalidCode) {
		t.Fatalf("Create error = %v, want ErrInvalidCode", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected SQL calls: %v", err)
	}
}

func TestShortLinkRepositoryFindByCode(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewShortLinkRepository(db)
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	code := link.NewShortCode(30)

	rows := sqlmock.NewRows([]string{
		"id", "code", "long_url", "domain", "expire_at", "status", "created_at", "updated_at",
	}).AddRow(uint64(30), code, "https://example.com/campaign/30", "sho.rt", nil, uint8(link.ShortLinkStatusActive), now, now)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `short_link_14` WHERE code = ? ORDER BY `short_link_14`.`id` LIMIT ?")).
		WithArgs(code, 1).
		WillReturnRows(rows)

	got, err := repo.FindByCode(context.Background(), code)
	if err != nil {
		t.Fatalf("FindByCode returned error: %v", err)
	}
	if got.ID != 30 || got.Code != code || got.LongURL != "https://example.com/campaign/30" {
		t.Fatalf("FindByCode returned unexpected item: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestShortLinkRepositoryFindByCodeNotFound(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewShortLinkRepository(db)
	code := link.NewShortCode(30)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `short_link_14` WHERE code = ? ORDER BY `short_link_14`.`id` LIMIT ?")).
		WithArgs(code, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "long_url", "domain", "expire_at", "status", "created_at", "updated_at"}))

	_, err := repo.FindByCode(context.Background(), code)
	if !errors.Is(err, link.ErrNotFound) {
		t.Fatalf("FindByCode error = %v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestShortLinkRepositoryFindActiveByCodeRejectsExpired(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewShortLinkRepository(db)
	repo.now = func() time.Time {
		return time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	}

	expireAt := repo.now().Add(-time.Minute)
	code := link.NewShortCode(29)
	rows := sqlmock.NewRows([]string{
		"id", "code", "long_url", "domain", "expire_at", "status", "created_at", "updated_at",
	}).AddRow(uint64(29), code, "https://example.com/campaign/29", "sho.rt", expireAt, uint8(link.ShortLinkStatusActive), repo.now(), repo.now())

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `short_link_13` WHERE code = ? ORDER BY `short_link_13`.`id` LIMIT ?")).
		WithArgs(code, 1).
		WillReturnRows(rows)

	_, err := repo.FindActiveByCode(context.Background(), code)
	if !errors.Is(err, link.ErrExpired) {
		t.Fatalf("FindActiveByCode error = %v, want ErrExpired", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestShortLinkRecordMapping(t *testing.T) {
	t.Parallel()

	expireAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	item := link.ShortLink{
		ID:        42,
		Code:      link.NewShortCode(42),
		LongURL:   "https://example.com/product/42",
		Domain:    "sho.rt",
		ExpireAt:  &expireAt,
		Status:    link.ShortLinkStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	record := shortLinkToRecord(item)
	if record.ID != item.ID || record.Code != item.Code || record.LongURL != item.LongURL {
		t.Fatalf("shortLinkToRecord lost core fields: %#v", record)
	}
	if record.Status != uint8(item.Status) {
		t.Fatalf("status = %d, want %d", record.Status, item.Status)
	}

	got := recordToShortLink(record)
	if got.ID != item.ID || got.Code != item.Code || got.LongURL != item.LongURL {
		t.Fatalf("recordToShortLink lost core fields: %#v", got)
	}
	if got.Status != item.Status {
		t.Fatalf("status = %d, want %d", got.Status, item.Status)
	}
}

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	return gormDB, mock, func() {
		_ = sqlDB.Close()
	}
}
