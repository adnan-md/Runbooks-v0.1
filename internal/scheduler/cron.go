package scheduler

import (
	"fmt"
	"strings"
	"time"
)

func nextRun(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}
	m, err := parseField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, err
	}
	h, err := parseField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, err
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, err
	}
	mon, err := parseField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, err
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, err
	}

	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 525600; i++ {
		if m[t.Minute()] && h[t.Hour()] && mon[int(t.Month())] && dom[t.Day()] && dow[int(t.Weekday())] {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching time in next year")
}

func parseField(s string, lo, hi int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			for i := lo; i <= hi; i++ {
				out[i] = true
			}
			continue
		}
		if part == "" {
			continue
		}
		step := 1
		if idx := strings.Index(part, "/"); idx >= 0 {
			n, err := atoi(part[idx+1:])
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid step %q", part)
			}
			step = n
			part = part[:idx]
		}
		var a, b int
		if part == "" || part == "*" {
			a, b = lo, hi
		} else if idx := strings.Index(part, "-"); idx >= 0 {
			n1, err := atoi(part[:idx])
			if err != nil {
				return nil, err
			}
			n2, err := atoi(part[idx+1:])
			if err != nil {
				return nil, err
			}
			a, b = n1, n2
		} else {
			n, err := atoi(part)
			if err != nil {
				return nil, err
			}
			a, b = n, n
		}
		if a < lo || b > hi || a > b {
			return nil, fmt.Errorf("range %d-%d out of bounds [%d,%d]", a, b, lo, hi)
		}
		for i := a; i <= b; i += step {
			out[i] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty cron field %q", s)
	}
	return out, nil
}

func atoi(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}
