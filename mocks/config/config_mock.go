// Code generated by MockGen. DO NOT EDIT.
// Source: config/config.go
//
// Generated by this command:
//
//	mockgen -source config/config.go -destination mocks/config/config_mock.go -package config
//

// Package config is a generated GoMock package.
package config

import (
	reflect "reflect"

	config "github.com/temporalio/s2s-proxy/config"
	gomock "go.uber.org/mock/gomock"
)

// MockConfigProvider is a mock of ConfigProvider interface.
type MockConfigProvider struct {
	ctrl     *gomock.Controller
	recorder *MockConfigProviderMockRecorder
}

// MockConfigProviderMockRecorder is the mock recorder for MockConfigProvider.
type MockConfigProviderMockRecorder struct {
	mock *MockConfigProvider
}

// NewMockConfigProvider creates a new mock instance.
func NewMockConfigProvider(ctrl *gomock.Controller) *MockConfigProvider {
	mock := &MockConfigProvider{ctrl: ctrl}
	mock.recorder = &MockConfigProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfigProvider) EXPECT() *MockConfigProviderMockRecorder {
	return m.recorder
}

// GetS2SProxyConfig mocks base method.
func (m *MockConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetS2SProxyConfig")
	ret0, _ := ret[0].(config.S2SProxyConfig)
	return ret0
}

// GetS2SProxyConfig indicates an expected call of GetS2SProxyConfig.
func (mr *MockConfigProviderMockRecorder) GetS2SProxyConfig() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetS2SProxyConfig", reflect.TypeOf((*MockConfigProvider)(nil).GetS2SProxyConfig))
}
