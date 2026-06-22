package bytesutil_test

import (
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestZeroRoot(t *testing.T) {
	input := make([]byte, fieldparams.RootLength)
	output := bytesutil.ZeroRoot(input)
	assert.Equal(t, true, output)
	copy(input[2:], "a")
	copy(input[3:], "b")
	output = bytesutil.ZeroRoot(input)
	assert.Equal(t, false, output)
}

func TestIsRoot(t *testing.T) {
	input := make([]byte, fieldparams.RootLength)
	output := bytesutil.IsRoot(input)
	assert.Equal(t, true, output)
}

func TestIsValidRoot(t *testing.T) {

	zeroRoot := make([]byte, fieldparams.RootLength)

	validRoot := make([]byte, fieldparams.RootLength)
	validRoot[0] = 'a'

	wrongLengthRoot := make([]byte, fieldparams.RootLength-4)
	wrongLengthRoot[0] = 'a'

	type args struct {
		root []byte
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Is ZeroRoot",
			args: args{
				root: zeroRoot,
			},
			want: false,
		},
		{
			name: "Is ValidRoot",
			args: args{
				root: validRoot,
			},
			want: true,
		},
		{
			name: "Is NonZeroRoot but not length 32",
			args: args{
				root: wrongLengthRoot,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bytesutil.IsValidRoot(tt.args.root)
			require.Equal(t, got, tt.want)
		})
	}
}
