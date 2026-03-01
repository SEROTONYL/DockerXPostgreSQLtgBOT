package economy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserFacingMessages_NoMojibakeMarkers(t *testing.T) {
	markers := []string{"Рќ", "РµСЂ", "РѕР»", "Р°Рґ", "PSP"}
	files := []string{
		"handlers.go",
		"service.go",
		filepath.Join("..", "casino", "handlers.go"),
		filepath.Join("..", "streak", "handlers.go"),
		filepath.Join("..", "streak", "service.go"),
	}

	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		for i, line := range strings.Split(string(b), "\n") {
			if !isUserFacingLine(line) {
				continue
			}
			for _, marker := range markers {
				if strings.Contains(line, marker) {
					t.Fatalf("mojibake marker %q in user-facing line %s:%d: %s", marker, file, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

func isUserFacingLine(line string) bool {
	return strings.Contains(line, "sendMessage(chatID") ||
		strings.Contains(line, "sb.WriteString(") ||
		strings.Contains(line, "text := fmt.Sprintf(") ||
		strings.Contains(line, "msg := fmt.Sprintf(") ||
		strings.Contains(line, "return \"")
}
