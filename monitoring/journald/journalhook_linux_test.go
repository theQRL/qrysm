package journald

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestJournalHook_LevelsRespectsVerbosity(t *testing.T) {
	cases := []struct {
		name  string
		level logrus.Level
		want  []logrus.Level
	}{
		{"warn drops info+debug", logrus.WarnLevel, logrus.AllLevels[:logrus.WarnLevel+1]},
		{"info drops debug", logrus.InfoLevel, logrus.AllLevels[:logrus.InfoLevel+1]},
		{"debug emits all", logrus.DebugLevel, logrus.AllLevels[:logrus.DebugLevel+1]},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := &JournalHook{level: tt.level}
			got := h.Levels()
			if len(got) != len(tt.want) {
				t.Fatalf("len(Levels()) = %d, want %d (%v vs %v)", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Levels()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStringifyEntries(t *testing.T) {
	input := map[string]any{
		"foo":     "bar",
		"baz":     123,
		"foo-foo": "x",
		"-bar":    "1",
	}

	output := stringifyEntries(input)
	if output["FOO"] != "bar" {
		t.Fatalf("%v", output)
		t.Fatalf("expected value 'bar'. Got %q", output["FOO"])
	}
	if output["BAZ"] != "123" {
		t.Fatalf("expected value '123'. Got %q", output["BAZ"])
	}
	if output["FOO_FOO"] != "x" {
		t.Fatalf("expected value 'x'. Got %q", output["FOO_FOO"])
	}
	if output["BAR"] != "1" {
		t.Fatalf("expected value 'x'. Got %q", output["BAR"])
	}
}
