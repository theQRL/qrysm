package v1

import (
	"fmt"
	"testing"
)

func TestFloorLog2(t *testing.T) {
	for _, tc := range []struct {
		value uint64
		want  int
	}{
		{0, -1}, {1, 0}, {2, 1}, {3, 1}, {4, 2}, {7, 2}, {8, 3},
		{55, 5}, {105, 6}, {1 << 32, 32}, {1 << 63, 63}, {^uint64(0), 63},
	} {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			if got := FloorLog2(tc.value); got != tc.want {
				t.Fatalf("FloorLog2(%d) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

func TestLightClientUpdate_EmptyBranches(t *testing.T) {
	for _, kind := range []struct {
		name     string
		depth    int
		isUpdate func([][]byte) bool
	}{
		{"sync committee", 5, func(branch [][]byte) bool {
			return (&LightClientUpdate{NextSyncCommitteeBranch: branch}).IsSyncCommiteeUpdate()
		}},
		{"finality", 6, func(branch [][]byte) bool {
			return (&LightClientUpdate{FinalityBranch: branch}).IsFinalityUpdate()
		}},
	} {
		t.Run(kind.name, func(t *testing.T) {
			branch := func(depth, width int) [][]byte {
				result := make([][]byte, depth)
				for i := range result {
					result[i] = make([]byte, width)
				}
				return result
			}
			firstNonzero := branch(kind.depth, 32)
			firstNonzero[0][0] = 1
			lastNonzero := branch(kind.depth, 32)
			lastNonzero[kind.depth-1][31] = 1
			for _, tc := range []struct {
				name   string
				branch [][]byte
				want   bool
			}{
				{"omitted", nil, false},
				{"empty slice", [][]byte{}, false},
				{"zero roots", branch(kind.depth, 32), false},
				{"nil roots", make([][]byte, kind.depth), false},
				{"empty roots", branch(kind.depth, 0), false},
				{"first nonzero root", firstNonzero, true},
				{"last nonzero root", lastNonzero, true},
				{"too few roots", branch(kind.depth-1, 32), true},
				{"too many roots", branch(kind.depth+1, 32), true},
				{"short root", branch(kind.depth, 31), true},
				{"long root", branch(kind.depth, 33), true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if got := kind.isUpdate(tc.branch); got != tc.want {
						t.Fatalf("isUpdate = %t, want %t", got, tc.want)
					}
				})
			}
		})
	}
}
