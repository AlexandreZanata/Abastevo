package domain

// SourceCollectionDay derives the calendar day of the verified native source
// clock without inventing a timezone. The caller retains the original clock for
// fingerprinting, sample scope and provenance. Only observed source formats are
// supported: date, or exactly YYYY-MM-DD HH:MM:SS (no zone/fraction guessing).
func SourceCollectionDay(raw string) (string, bool) {
	if len(raw) == 10 {
		return raw, ValidDay(raw)
	}
	if len(raw) != 19 || raw[10] != ' ' || raw[13] != ':' || raw[16] != ':' {
		return "", false
	}
	for _, i := range []int{11, 12, 14, 15, 17, 18} {
		if raw[i] < '0' || raw[i] > '9' {
			return "", false
		}
	}
	if raw[11:13] > "23" || raw[14:16] > "59" || raw[17:19] > "59" || !ValidDay(raw[:10]) {
		return "", false
	}
	return raw[:10], true
}
