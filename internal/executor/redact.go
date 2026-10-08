package executor

import (
	"io"
	"strings"
)

type RedactingWriter struct {
	W        io.Writer
	Secrets  []string
	buf      []byte
}

func NewRedactingWriter(w io.Writer, secrets []string) *RedactingWriter {
	var s []string
	for _, v := range secrets {
		v = strings.TrimSpace(v)
		if v != "" {
			s = append(s, v)
		}
	}
	return &RedactingWriter{W: w, Secrets: s}
}

func (r *RedactingWriter) Write(p []byte) (int, error) {
	out := p
	for _, s := range r.Secrets {
		if s == "" {
			continue
		}
		out = bytesReplaceAll(out, []byte(s), []byte("[REDACTED]"))
	}
	n, err := r.W.Write(out)
	if n > len(p) {
		n = len(p)
	}
	return n, err
}

func bytesReplaceAll(s, old, new []byte) []byte {
	if len(old) == 0 {
		return s
	}
	var out []byte
	i := 0
	for i < len(s) {
		j := indexBytes(s[i:], old)
		if j < 0 {
			out = append(out, s[i:]...)
			break
		}
		out = append(out, s[i:i+j]...)
		out = append(out, new...)
		i = i + j + len(old)
	}
	return out
}

func indexBytes(s, sub []byte) int {
	if len(sub) == 0 || len(s) < len(sub) {
		return -1
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
