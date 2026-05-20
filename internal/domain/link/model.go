package link

import "time"

type ShortLinkStatus uint8

const (
	ShortLinkStatusActive ShortLinkStatus = 1
	ShortLinkStatusBanned ShortLinkStatus = 2
)

type ShortLink struct {
	ID        uint64
	Code      string
	LongURL   string
	Domain    string
	ExpireAt  *time.Time
	Status    ShortLinkStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (l ShortLink) IsExpired(now time.Time) bool {
	return l.ExpireAt != nil && !l.ExpireAt.After(now)
}

func (l ShortLink) IsActive(now time.Time) bool {
	return l.Status == ShortLinkStatusActive && !l.IsExpired(now)
}

type VisitLog struct {
	ID        uint64
	Code      string
	VisitedAt time.Time
	IP        string
	UserAgent string
	Referer   string
}

type RefererStat struct {
	Referer string `json:"referer"`
	PV      uint64 `json:"pv"`
}

type LinkStats struct {
	Code      string        `json:"code"`
	PV        uint64        `json:"pv"`
	UV        uint64        `json:"uv"`
	TodayPV   uint64        `json:"today_pv"`
	Referers  []RefererStat `json:"referers"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type DailyStat struct {
	Code      string
	StatDate  time.Time
	PV        uint64
	UV        uint64
	CreatedAt time.Time
	UpdatedAt time.Time
}
