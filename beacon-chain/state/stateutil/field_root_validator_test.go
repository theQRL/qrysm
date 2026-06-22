package stateutil

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	mathutil "github.com/theQRL/qrysm/math"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestValidatorConstants(t *testing.T) {
	v := &qrysmpb.Validator{}
	refV := reflect.ValueOf(v).Elem()
	numFields := refV.NumField()
	numOfValFields := 0

	for i := range numFields {
		if strings.Contains(refV.Type().Field(i).Name, "state") ||
			strings.Contains(refV.Type().Field(i).Name, "sizeCache") ||
			strings.Contains(refV.Type().Field(i).Name, "unknownFields") {
			continue
		}
		numOfValFields++
	}
	assert.Equal(t, validatorFieldRoots, numOfValFields)
	assert.Equal(t, uint64(validatorFieldRoots), mathutil.PowerOf2(validatorTreeDepth))

	_, err := ValidatorRegistryRoot([]*qrysmpb.Validator{v})
	assert.NoError(t, err)
}

func TestHashValidatorHelper(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	v := &qrysmpb.Validator{}
	valList := make([]*qrysmpb.Validator, 10*validatorFieldRoots)
	for i := range valList {
		valList[i] = v
	}
	roots := make([][32]byte, len(valList))
	hashValidatorHelper(valList, roots, 2, 2, &wg)
	for i := range 4 * validatorFieldRoots {
		require.Equal(t, [32]byte{}, roots[i])
	}
	emptyValRoots, err := ValidatorFieldRoots(v)
	require.NoError(t, err)
	for i := 4; i < 6; i++ {
		for j := range validatorFieldRoots {
			require.Equal(t, emptyValRoots[j], roots[i*validatorFieldRoots+j])
		}
	}
	for i := 6 * validatorFieldRoots; i < 10*validatorFieldRoots; i++ {
		require.Equal(t, [32]byte{}, roots[i])
	}
}
