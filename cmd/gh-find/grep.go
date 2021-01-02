package main

import (
	"bufio"
	"io"
	"regexp"
)

type grepMatch struct {
	line   string
	lineno int64
}

type grepResults struct {
	isBinary bool
	matches  []grepMatch
}

func grep(contents io.Reader, pattern *regexp.Regexp, limit int) (*grepResults, error) {
	if contents == nil || pattern == nil {
		return &grepResults{}, nil
	}

	reader := bufio.NewReader(contents)
	chunk, _ := reader.Peek(256)
	for i := 0; i < len(chunk); i++ {
		if chunk[i] == 0 {
			return &grepResults{isBinary: true}, nil // Skip if the contents is binary.
		}
	}
	chunk = nil

	var (
		lineno  int64
		results = &grepResults{}
	)
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		if limit > 0 && len(results.matches) >= limit {
			break
		}
		lineno++
		if pattern.Match(scanner.Bytes()) {
			results.matches = append(results.matches, grepMatch{line: scanner.Text(), lineno: lineno})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
