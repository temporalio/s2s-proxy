// Code generated by MockGen. DO NOT EDIT.
// Source: client/temporal_client.go
//
// Generated by this command:
//
//	mockgen -source client/temporal_client.go -destination mocks/client/temporal_client_mock.go -package client
//

// Package client is a generated GoMock package.
package client

import (
	reflect "reflect"

	config "github.com/temporalio/s2s-proxy/config"
	operatorservice "go.temporal.io/api/operatorservice/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	gomock "go.uber.org/mock/gomock"
)

// MockClientFactory is a mock of ClientFactory interface.
type MockClientFactory struct {
	ctrl     *gomock.Controller
	recorder *MockClientFactoryMockRecorder
}

// MockClientFactoryMockRecorder is the mock recorder for MockClientFactory.
type MockClientFactoryMockRecorder struct {
	mock *MockClientFactory
}

// NewMockClientFactory creates a new mock instance.
func NewMockClientFactory(ctrl *gomock.Controller) *MockClientFactory {
	mock := &MockClientFactory{ctrl: ctrl}
	mock.recorder = &MockClientFactoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClientFactory) EXPECT() *MockClientFactoryMockRecorder {
	return m.recorder
}

// NewRemoteAdminClient mocks base method.
func (m *MockClientFactory) NewRemoteAdminClient() (adminservice.AdminServiceClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewRemoteAdminClient")
	ret0, _ := ret[0].(adminservice.AdminServiceClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewRemoteAdminClient indicates an expected call of NewRemoteAdminClient.
func (mr *MockClientFactoryMockRecorder) NewRemoteAdminClient() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewRemoteAdminClient", reflect.TypeOf((*MockClientFactory)(nil).NewRemoteAdminClient))
}

// NewRemoteOperatorClient mocks base method.
func (m *MockClientFactory) NewRemoteOperatorClient() (operatorservice.OperatorServiceClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewRemoteOperatorClient")
	ret0, _ := ret[0].(operatorservice.OperatorServiceClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewRemoteOperatorClient indicates an expected call of NewRemoteOperatorClient.
func (mr *MockClientFactoryMockRecorder) NewRemoteOperatorClient() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewRemoteOperatorClient", reflect.TypeOf((*MockClientFactory)(nil).NewRemoteOperatorClient))
}

// NewRemoteWorkflowServiceClient mocks base method.
func (m *MockClientFactory) NewRemoteWorkflowServiceClient(clientConfig config.ProxyClientConfig) (workflowservice.WorkflowServiceClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewRemoteWorkflowServiceClient", clientConfig)
	ret0, _ := ret[0].(workflowservice.WorkflowServiceClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewRemoteWorkflowServiceClient indicates an expected call of NewRemoteWorkflowServiceClient.
func (mr *MockClientFactoryMockRecorder) NewRemoteWorkflowServiceClient(clientConfig any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewRemoteWorkflowServiceClient", reflect.TypeOf((*MockClientFactory)(nil).NewRemoteWorkflowServiceClient), clientConfig)
}

// MockClientProvider is a mock of ClientProvider interface.
type MockClientProvider struct {
	ctrl     *gomock.Controller
	recorder *MockClientProviderMockRecorder
}

// MockClientProviderMockRecorder is the mock recorder for MockClientProvider.
type MockClientProviderMockRecorder struct {
	mock *MockClientProvider
}

// NewMockClientProvider creates a new mock instance.
func NewMockClientProvider(ctrl *gomock.Controller) *MockClientProvider {
	mock := &MockClientProvider{ctrl: ctrl}
	mock.recorder = &MockClientProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClientProvider) EXPECT() *MockClientProviderMockRecorder {
	return m.recorder
}

// GetAdminClient mocks base method.
func (m *MockClientProvider) GetAdminClient() (adminservice.AdminServiceClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAdminClient")
	ret0, _ := ret[0].(adminservice.AdminServiceClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAdminClient indicates an expected call of GetAdminClient.
func (mr *MockClientProviderMockRecorder) GetAdminClient() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAdminClient", reflect.TypeOf((*MockClientProvider)(nil).GetAdminClient))
}

// GetOperatorClient mocks base method.
func (m *MockClientProvider) GetOperatorClient() (operatorservice.OperatorServiceClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOperatorClient")
	ret0, _ := ret[0].(operatorservice.OperatorServiceClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOperatorClient indicates an expected call of GetOperatorClient.
func (mr *MockClientProviderMockRecorder) GetOperatorClient() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOperatorClient", reflect.TypeOf((*MockClientProvider)(nil).GetOperatorClient))
}

// GetWorkflowServiceClient mocks base method.
func (m *MockClientProvider) GetWorkflowServiceClient() (workflowservice.WorkflowServiceClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWorkflowServiceClient")
	ret0, _ := ret[0].(workflowservice.WorkflowServiceClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetWorkflowServiceClient indicates an expected call of GetWorkflowServiceClient.
func (mr *MockClientProviderMockRecorder) GetWorkflowServiceClient() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWorkflowServiceClient", reflect.TypeOf((*MockClientProvider)(nil).GetWorkflowServiceClient))
}
