package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"golang.org/x/sync/errgroup"
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

func (r *ShortLinkRepository) CreateBatch(ctx context.Context, items []link.ShortLink) error {
	if len(items) == 0 {
		return nil
	}

	groups, err := groupShortLinksByShard(items)
	if err != nil {
		return err
	}

	group, ctx := errgroup.WithContext(ctx)
	for shard, records := range groups {
		if len(records) == 0 {
			continue
		}

		shard := shard
		records := records
		group.Go(func() error {
			tableName, err := link.ShardTableName(shard)
			if err != nil {
				return err
			}

			return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return tx.Table(tableName).CreateInBatches(records, len(records)).Error
			})
		})
	}

	return group.Wait()
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

func (r *ShortLinkRepository) ListActiveCodes(ctx context.Context, batchSize int, handle func([]string) error) error {
	if batchSize <= 0 {
		batchSize = 1000
	}

	now := r.now()
	for shard := 0; shard < link.ShardCount; shard++ {
		tableName, err := link.ShardTableName(shard)
		if err != nil {
			return err
		}

		var lastID uint64
		for {
			if err := ctx.Err(); err != nil {
				return err
			}

			var rows []activeCodeRow
			if err := r.db.WithContext(ctx).
				Table(tableName).
				Select("id, code").
				Where("id > ? AND status = ? AND (expire_at IS NULL OR expire_at > ?)", lastID, uint8(link.ShortLinkStatusActive), now).
				Order("id ASC").
				Limit(batchSize).
				Find(&rows).
				Error; err != nil {
				return err
			}

			if len(rows) == 0 {
				break
			}

			codes := make([]string, 0, len(rows))
			for _, row := range rows {
				codes = append(codes, row.Code)
			}
			if err := handle(codes); err != nil {
				return err
			}

			lastID = rows[len(rows)-1].ID
			if len(rows) < batchSize {
				break
			}
		}
	}

	return nil
}

func (r *ShortLinkRepository) DeleteExpired(ctx context.Context, before time.Time, batchSize int, handleCodes func([]string) error) (int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}

	var total int64
	for shard := 0; shard < link.ShardCount; shard++ {
		tableName, err := link.ShardTableName(shard)
		if err != nil {
			return total, err
		}

		var cursorExpireAt time.Time
		var cursorID uint64
		for {
			if err := ctx.Err(); err != nil {
				return total, err
			}

			var rows []expiredCodeRow
			query := r.db.WithContext(ctx).
				Table(tableName).
				Select("id, code, expire_at").
				Where("expire_at IS NOT NULL AND expire_at <= ?", before)
			if !cursorExpireAt.IsZero() {
				query = query.Where("(expire_at > ? OR (expire_at = ? AND id > ?))", cursorExpireAt, cursorExpireAt, cursorID)
			}
			if err := query.
				Order("expire_at ASC, id ASC").
				Limit(batchSize).
				Find(&rows).
				Error; err != nil {
				return total, err
			}
			if len(rows) == 0 {
				break
			}

			ids := make([]uint64, 0, len(rows))
			codes := make([]string, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.ID)
				codes = append(codes, row.Code)
			}
			if handleCodes != nil {
				if err := handleCodes(codes); err != nil {
					return total, err
				}
			}

			result := r.db.WithContext(ctx).
				Table(tableName).
				Where("id IN ?", ids).
				Delete(&ShortLinkRecord{})
			if result.Error != nil {
				return total, result.Error
			}
			total += result.RowsAffected

			last := rows[len(rows)-1]
			cursorExpireAt = last.ExpireAt
			cursorID = last.ID

			if len(rows) < batchSize {
				break
			}
		}
	}

	return total, nil
}

func (r *ShortLinkRepository) DeleteExpiredCode(ctx context.Context, code string, before time.Time) (bool, error) {
	if err := link.ValidateShortCode(code); err != nil {
		return false, err
	}

	tableName, err := link.ShardTableNameForCode(code)
	if err != nil {
		return false, err
	}

	result := r.db.WithContext(ctx).
		Table(tableName).
		Where("code = ? AND expire_at IS NOT NULL AND expire_at <= ?", code, before).
		Delete(&ShortLinkRecord{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

type activeCodeRow struct {
	ID   uint64 `gorm:"column:id"`
	Code string `gorm:"column:code"`
}

type expiredCodeRow struct {
	ID       uint64    `gorm:"column:id"`
	Code     string    `gorm:"column:code"`
	ExpireAt time.Time `gorm:"column:expire_at"`
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

func groupShortLinksByShard(items []link.ShortLink) ([link.ShardCount][]ShortLinkRecord, error) {
	var groups [link.ShardCount][]ShortLinkRecord
	for _, item := range items {
		if err := link.ValidateShortCode(item.Code); err != nil {
			return groups, err
		}

		shard, err := link.ShardIndexFromCode(item.Code)
		if err != nil {
			return groups, err
		}
		groups[shard] = append(groups[shard], shortLinkToRecord(item))
	}
	return groups, nil
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
