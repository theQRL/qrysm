package version

import "testing"

func TestVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version int
		want    string
	}{
		{
			name:    "capella",
			version: Capella,
			want:    "capella",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.version); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
