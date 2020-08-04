package utils

// StringContains check if an array contains a specific value
func StringContains(array []string, needle string) bool {
	for _, val := range array {
		if val == needle {
			return true
		}
	}
	return false
}
