package cache

const keyPrefix = "flashlink"

func LinkKey(code string) string {
	return keyPrefix + ":link:" + code
}
