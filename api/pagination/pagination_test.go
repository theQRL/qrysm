package pagination_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/theQRL/qrysm/api/pagination"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStartAndEndPage(t *testing.T) {
	tests := []struct {
		token     string
		pageSize  int
		totalSize int
		nextToken string
		start     int
		end       int
	}{
		{
			token:     "0",
			pageSize:  9,
			totalSize: 100,
			nextToken: "1",
			start:     0,
			end:       9,
		},
		{
			token:     "10",
			pageSize:  4,
			totalSize: 100,
			nextToken: "11",
			start:     40,
			end:       44,
		},
		{
			token:     "100",
			pageSize:  5,
			totalSize: 1000,
			nextToken: "101",
			start:     500,
			end:       505,
		},
		{
			token:     "3",
			pageSize:  33,
			totalSize: 100,
			nextToken: "",
			start:     99,
			end:       100,
		},
		{
			token:     "34",
			pageSize:  500,
			totalSize: 17500,
			nextToken: "",
			start:     17000,
			end:       17500,
		},
	}

	for _, test := range tests {
		start, end, next, err := pagination.StartAndEndPage(test.token, test.pageSize, test.totalSize)
		require.NoError(t, err)
		if test.start != start {
			t.Errorf("expected start and computed start are not equal %d, %d", test.start, start)
		}
		if test.end != end {
			t.Errorf("expected end and computed end are not equal %d, %d", test.end, end)
		}
		if test.nextToken != next {
			t.Errorf("expected next token and computed next token are not equal %v, %v", test.nextToken, next)
		}
	}
}

func TestStartAndEndPage_CannotConvertPage(t *testing.T) {
	wanted := "could not convert page token: strconv.Atoi: parsing"
	_, _, _, err := pagination.StartAndEndPage("bad", 0, 0)
	assert.ErrorContains(t, wanted, err)
}

func TestStartAndEndPage_ExceedsMaxPage(t *testing.T) {
	wanted := "page start 0 >= list 0"
	_, _, _, err := pagination.StartAndEndPage("", 0, 0)
	assert.ErrorContains(t, wanted, err)
}

func TestStartAndEndPage_InvalidPageValues(t *testing.T) {
	_, _, _, err := pagination.StartAndEndPage("10", -1, 10)
	assert.ErrorContains(t, "invalid page and total sizes provided", err)

	_, _, _, err = pagination.StartAndEndPage("12", 10, -10)
	assert.ErrorContains(t, "invalid page and total sizes provided", err)

	_, _, _, err = pagination.StartAndEndPage("12", -50, -60)
	assert.ErrorContains(t, "invalid page and total sizes provided", err)
}

func TestStartAndEndPage_InvalidTokenValue(t *testing.T) {
	_, _, _, err := pagination.StartAndEndPage("-12", 50, 60)
	assert.ErrorContains(t, "invalid token value provided", err)
}

// TestStartAndEndPage_TokenTimesPageSizeOverflow is a regression test for
// token*pageSize wrapping around: both values come straight from the request,
// and a wrapped (negative) start used to be handed back without error, which
// turned into a slice-bounds panic in every paginated gRPC handler.
func TestStartAndEndPage_TokenTimesPageSizeOverflow(t *testing.T) {
	huge := strconv.Itoa(math.MaxInt/2 + 1)

	_, _, _, err := pagination.StartAndEndPage(huge, 2, 10)
	assert.ErrorContains(t, "overflows", err)

	_, _, _, err = pagination.StartAndEndPage(strconv.Itoa(math.MaxInt), 0 /* default page size */, 10)
	assert.ErrorContains(t, "overflows", err)

	_, _, _, err = pagination.StartAndEndPage("3", math.MaxInt, 10)
	assert.ErrorContains(t, "overflows", err)

	// The largest token that does not overflow is still handled: it simply
	// points past the end of the list.
	pageSize := 100
	maxToken := (math.MaxInt - pageSize) / pageSize
	_, _, _, err = pagination.StartAndEndPage(strconv.Itoa(maxToken), pageSize, 10)
	assert.ErrorContains(t, ">= list 10", err)

	// A single page that spans the whole int range is fine.
	start, end, next, err := pagination.StartAndEndPage("0", math.MaxInt, 10)
	assert.NoError(t, err)
	assert.Equal(t, 0, start)
	assert.Equal(t, 10, end)
	assert.Equal(t, "", next)
}
