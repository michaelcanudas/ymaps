package env

import (
	"bufio"
	"os"
	"strings"
)

func Load(paths ...string) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			key = strings.TrimSpace(key)
			if _, set := os.LookupEnv(key); set {
				continue
			}
			os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`))
		}
		file.Close()
	}
}
