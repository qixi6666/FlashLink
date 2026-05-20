package link

import "fmt"

const ShardCount = 16

func ShardIndexFromID(id uint64) int {
	return int(id % ShardCount)
}

func ShardIndexFromCode(code string) (int, error) {
	id, err := IDFromShortCode(code)
	if err != nil {
		return 0, err
	}
	return ShardIndexFromID(id), nil
}

func ShardTableName(index int) (string, error) {
	if index < 0 || index >= ShardCount {
		return "", fmt.Errorf("shard index %d out of range", index)
	}
	return fmt.Sprintf("short_link_%02d", index), nil
}

func ShardTableNameForCode(code string) (string, error) {
	idx, err := ShardIndexFromCode(code)
	if err != nil {
		return "", err
	}
	return ShardTableName(idx)
}
