CREATE TABLE IF NOT EXISTS short_link_00 (
  id BIGINT UNSIGNED NOT NULL,
  code VARCHAR(16) NOT NULL,
  long_url VARCHAR(2048) NOT NULL,
  domain VARCHAR(128) NOT NULL DEFAULT '',
  expire_at DATETIME(3) NULL,
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_short_link_00_code (code),
  KEY idx_short_link_00_expire_at (expire_at, id),
  KEY idx_short_link_00_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS short_link_01 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_02 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_03 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_04 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_05 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_06 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_07 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_08 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_09 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_10 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_11 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_12 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_13 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_14 LIKE short_link_00;
CREATE TABLE IF NOT EXISTS short_link_15 LIKE short_link_00;

CREATE TABLE IF NOT EXISTS visit_log (
  id BIGINT UNSIGNED NOT NULL,
  code VARCHAR(16) NOT NULL,
  visited_at DATETIME(3) NOT NULL,
  ip VARCHAR(45) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  referer VARCHAR(2048) NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  KEY idx_visit_log_code_visited_at (code, visited_at),
  KEY idx_visit_log_visited_at (visited_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS link_stat_daily (
  id BIGINT UNSIGNED NOT NULL,
  code VARCHAR(16) NOT NULL,
  stat_date DATE NOT NULL,
  pv BIGINT UNSIGNED NOT NULL DEFAULT 0,
  uv BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_link_stat_daily_code_date (code, stat_date),
  KEY idx_link_stat_daily_stat_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
