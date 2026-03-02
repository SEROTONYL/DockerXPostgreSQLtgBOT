package jobs

import (
	"strings"
	"testing"
)

func TestCronLogMessages_DoNotContainMojibakeMarkers(t *testing.T) {
	messages := []string{
		cronWarnLoadLocation,
		cronInfoDailyReset,
		cronErrorDailyReset,
		cronDebugReminders,
		cronErrorReminders,
		cronInfoStarted,
		cronInfoStopped,
	}

	markers := []string{"Рќ", "РџСЂ", "РµСЂ", "РѕР»", "Р°Рґ", "PSP", "РѕР±", "РёСЃ"}

	for _, msg := range messages {
		for _, marker := range markers {
			if strings.Contains(msg, marker) {
				t.Fatalf("log message %q contains mojibake marker %q", msg, marker)
			}
		}
	}
}
