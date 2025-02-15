// Code generated by MockGen. DO NOT EDIT.
// Source: validator/client/beacon-api/genesis.go

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/eth/beacon"
	apimiddleware "github.com/theQRL/qrysm/v4/api/gateway/apimiddleware"
)

// MockgenesisProvider is a mock of genesisProvider interface.
type MockgenesisProvider struct {
	ctrl     *gomock.Controller
	recorder *MockgenesisProviderMockRecorder
}

// MockgenesisProviderMockRecorder is the mock recorder for MockgenesisProvider.
type MockgenesisProviderMockRecorder struct {
	mock *MockgenesisProvider
}

// NewMockgenesisProvider creates a new mock instance.
func NewMockgenesisProvider(ctrl *gomock.Controller) *MockgenesisProvider {
	mock := &MockgenesisProvider{ctrl: ctrl}
	mock.recorder = &MockgenesisProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockgenesisProvider) EXPECT() *MockgenesisProviderMockRecorder {
	return m.recorder
}

// GetGenesis mocks base method.
func (m *MockgenesisProvider) GetGenesis(ctx context.Context) (*beacon.Genesis, *apimiddleware.DefaultErrorJson, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetGenesis", ctx)
	ret0, _ := ret[0].(*beacon.Genesis)
	ret1, _ := ret[1].(*apimiddleware.DefaultErrorJson)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetGenesis indicates an expected call of GetGenesis.
func (mr *MockgenesisProviderMockRecorder) GetGenesis(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetGenesis", reflect.TypeOf((*MockgenesisProvider)(nil).GetGenesis), ctx)
}
