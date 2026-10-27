package selfupdate

import (
	"errors"
	"regexp"
	"strings"
)

var semverPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$`)

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func compareNumber(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func compareVersions(a, b string) (int, error) {
	x, y := semverPattern.FindStringSubmatch(a), semverPattern.FindStringSubmatch(b)
	if x == nil || y == nil {
		return 0, errors.New("invalid version")
	}
	for _, m := range [][]string{x, y} {
		if m[5] != "" {
			for _, p := range strings.Split(m[5], ".") {
				if numeric(p) && len(p) > 1 && p[0] == '0' {
					return 0, errors.New("invalid version")
				}
			}
		}
	}
	for i := 1; i <= 3; i++ {
		if n := compareNumber(x[i], y[i]); n != 0 {
			return n, nil
		}
	}
	if x[5] == y[5] {
		return 0, nil
	}
	if x[5] == "" {
		return 1, nil
	}
	if y[5] == "" {
		return -1, nil
	}
	p, q := strings.Split(x[5], "."), strings.Split(y[5], ".")
	for i := 0; i < len(p) && i < len(q); i++ {
		if p[i] == q[i] {
			continue
		}
		pn, qn := numeric(p[i]), numeric(q[i])
		if pn && qn {
			return compareNumber(p[i], q[i]), nil
		}
		if pn {
			return -1, nil
		}
		if qn {
			return 1, nil
		}
		return strings.Compare(p[i], q[i]), nil
	}
	if len(p) < len(q) {
		return -1, nil
	}
	return 1, nil
}
