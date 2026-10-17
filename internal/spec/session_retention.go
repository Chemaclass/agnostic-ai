package spec

func SessionRetentionDays(value any) (uint64, bool) {
	switch days := value.(type) {
	case int:
		if days > 0 {
			return uint64(days), true
		}
	case int64:
		if days > 0 {
			return uint64(days), true
		}
	case uint64:
		return days, days > 0
	}
	return 0, false
}
