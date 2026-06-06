package search

// levenshtein computes the edit distance between two strings.
// Uses an optimised single-row DP approach to minimise allocations.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	ra := []rune(a)
	rb := []rune(b)
	la := len(ra)
	lb := len(rb)

	// Keep only two rows.
	row := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		row[j] = j
	}

	for i := 1; i <= la; i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= lb; j++ {
			tmp := row[j]
			if ra[i-1] == rb[j-1] {
				row[j] = prev
			} else {
				row[j] = 1 + min3(prev, row[j], row[j-1])
			}
			prev = tmp
		}
	}
	return row[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
