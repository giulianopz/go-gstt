package str

func GetOrDefault(val, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}
