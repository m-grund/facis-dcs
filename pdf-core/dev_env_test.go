package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	loadDevEnvForTests()
	os.Exit(m.Run())
}

func loadDevEnvForTests() {
	f, err := os.Open(".dev.env")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
