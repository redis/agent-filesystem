package client

// globMatch matches a string against a glob pattern, following the same
// semantics as the C module's fsGlobMatch():
//   - * matches any sequence of characters (including empty)
//   - ? matches exactly one character
//   - [abc] matches one of a, b, or c
//   - [a-z] matches any character in range a-z inclusive
//   - [!x] or [^x] matches any character NOT in the set
//   - \x matches the literal character x
func globMatch(pattern, str string) bool {
	px, sx := 0, 0
	starPx, starSx := -1, -1

	for sx < len(str) {
		if px < len(pattern) {
			switch pattern[px] {
			case '*':
				starPx = px
				starSx = sx
				px++
				continue
			case '?':
				px++
				sx++
				continue
			case '[':
				if matched, newPx := matchClass(pattern, px, str[sx]); matched {
					px = newPx
					sx++
					continue
				}
			case '\\':
				px++
				if px < len(pattern) && pattern[px] == str[sx] {
					px++
					sx++
					continue
				}
			default:
				if pattern[px] == str[sx] {
					px++
					sx++
					continue
				}
			}
		}
		// No match at current position; backtrack to last *
		if starPx >= 0 {
			px = starPx + 1
			starSx++
			sx = starSx
			continue
		}
		return false
	}

	// Consume trailing *'s
	for px < len(pattern) && pattern[px] == '*' {
		px++
	}
	return px == len(pattern)
}

// matchClass handles a [...] character class at pattern[px].
// Returns (matched, newPx) where newPx points past the closing ].
func matchClass(pattern string, px int, ch byte) (bool, int) {
	if px >= len(pattern) || pattern[px] != '[' {
		return false, px
	}
	px++ // skip '['

	negate := false
	if px < len(pattern) && (pattern[px] == '!' || pattern[px] == '^') {
		negate = true
		px++
	}

	matched := false
	first := true

	for px < len(pattern) {
		if pattern[px] == ']' && !first {
			px++ // skip ']'
			if negate {
				return !matched, px
			}
			return matched, px
		}
		first = false

		c := pattern[px]
		if c == '\\' && px+1 < len(pattern) {
			px++
			c = pattern[px]
		}
		px++

		// Check for range: c-d
		if px+1 < len(pattern) && pattern[px] == '-' && pattern[px+1] != ']' {
			px++ // skip '-'
			d := pattern[px]
			if d == '\\' && px+1 < len(pattern) {
				px++
				d = pattern[px]
			}
			px++

			lo, hi := c, d
			if lo > hi {
				lo, hi = hi, lo
			}
			if ch >= lo && ch <= hi {
				matched = true
			}
		} else {
			if ch == c {
				matched = true
			}
		}
	}

	// Unterminated [ — treat as literal non-match
	return false, px
}
