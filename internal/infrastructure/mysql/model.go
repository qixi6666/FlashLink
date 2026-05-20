package mysql

import "time"

type ShortLinkRecord struct {
	ID        uint64     `gorm:"column:id;primaryKey;autoIncrement:false"`
	Code      string     `gorm:"column:code"`
	LongURL   string     `gorm:"column:long_url"`
	Domain    string     `gorm:"column:domain"`
	ExpireAt  *time.Time `gorm:"column:expire_at"`
	Status    uint8      `gorm:"column:status"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

type VisitLogRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement:false"`
	Code      string    `gorm:"column:code"`
	VisitedAt time.Time `gorm:"column:visited_at"`
	IP        string    `gorm:"column:ip"`
	UserAgent string    `gorm:"column:user_agent"`
	Referer   string    `gorm:"column:referer"`
}

func (VisitLogRecord) TableName() string {
	return "visit_log"
}

type DailyStatRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement:false"`
	Code      string    `gorm:"column:code"`
	StatDate  time.Time `gorm:"column:stat_date"`
	PV        uint64    `gorm:"column:pv"`
	UV        uint64    `gorm:"column:uv"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (DailyStatRecord) TableName() string {
	return "link_stat_daily"
}

type refererStatRecord struct {
	Referer string `gorm:"column:referer"`
	PV      uint64 `gorm:"column:pv"`
}
