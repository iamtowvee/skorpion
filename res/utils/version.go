package utils

import (
	"strconv"
	"strings"
)

// CompareVersions сравнивает две версии.
// Возвращает -1, если a < b; 0, если равно; 1, если a > b.
// Суффиксы после '-' (например "-dev.123") отбрасываются.
//
// Раньше существовала как res/skorpion.go::compareVersions.
func CompareVersions(a, b string) int {
	aParts := SplitVersion(a)
	bParts := SplitVersion(b)

	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// SplitVersion разбирает "0.13.0-dev.123" → [0, 13, 0].
func SplitVersion(v string) []int {
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	return nums
}
