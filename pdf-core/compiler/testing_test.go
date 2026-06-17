package compiler

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	loadDevEnv()
	os.Exit(m.Run())
}

func loadDevEnv() {
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
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if key != "" {
			_ = os.Setenv(key, value)
		}
	}
}
