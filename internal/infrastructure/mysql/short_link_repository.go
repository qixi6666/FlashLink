package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"gorm.io/gorm"
)

type ShortLinkRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewShortLinkRepository(db *gorm.DB) *ShortLinkRepository {
	return &ShortLinkRepository{
		db:  db,
		now: time.Now,
	}
}

func (r *ShortLinkRepository) Create(ctx context.Context, item link.ShortLink) error {
	if err := link.ValidateShortCode(item.Code); err != nil {
		return err
	}

	tableName, err := link.ShardTableNameForCode(item.Code)
	if err != nil {
		return err
	}

	record := shortLinkToRecord(item)
	return r.db.WithContext(ctx).Table(tableName).Create(&record).Error
}

func (r *ShortLinkRepository) FindByCode(ctx context.Context, code string) (link.ShortLink, error) {
	if err := link.ValidateShortCode(code); err != nil {
		return link.ShortLink{}, err
	}

	tableName, err := link.ShardTableNameForCode(code)
	if err != nil {
		return link.ShortLink{}, err
	}

	var record ShortLinkRecord
	err = r.db.WithContext(ctx).
		Table(tableName).
		Where("code = ?", code).
		First(&record).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return link.ShortLink{}, link.ErrNotFound
	}
	if err != nil {
		return link.ShortLink{}, err
	}

	return recordToShortLink(record), nil
}

func (r *ShortLinkRepository) FindActiveByCode(ctx context.Context, code string) (link.ShortLink, error) {
	item, err := r.FindByCode(ctx, code)
	if err != nil {
		return link.ShortLink{}, err
	}
	if item.IsExpired(r.now()) {
		return link.ShortLink{}, link.ErrExpired
	}
	if item.Status != link.ShortLinkStatusActive {
		return link.ShortLink{}, link.ErrNotFound
	}
	return item, nil
}

func shortLinkToRecord(item link.ShortLink) ShortLinkRecord {
	return ShortLinkRecord{
		ID:        item.ID,
		Code:      item.Code,
		LongURL:   item.LongURL,
		Domain:    item.Domain,
		ExpireAt:  item.ExpireAt,
		Status:    uint8(item.Status),
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func recordToShortLink(record ShortLinkRecord) link.ShortLink {
	return link.ShortLink{
		ID:        record.ID,
		Code:      record.Code,
		LongURL:   record.LongURL,
		Domain:    record.Domain,
		ExpireAt:  record.ExpireAt,
		Status:    link.ShortLinkStatus(record.Status),
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}
