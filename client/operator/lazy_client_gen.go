// Code generated by cmd/tools/genrpcwrappers. DO NOT EDIT.

package operator

import (
	"context"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
)

func (c *lazyClient) AddOrUpdateRemoteCluster(
	ctx context.Context,
	request *operatorservice.AddOrUpdateRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*operatorservice.AddOrUpdateRemoteClusterResponse, error) {
	var resp *operatorservice.AddOrUpdateRemoteClusterResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.AddOrUpdateRemoteCluster(ctx, request, opts...)
}

func (c *lazyClient) AddSearchAttributes(
	ctx context.Context,
	request *operatorservice.AddSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*operatorservice.AddSearchAttributesResponse, error) {
	var resp *operatorservice.AddSearchAttributesResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.AddSearchAttributes(ctx, request, opts...)
}

func (c *lazyClient) CreateNexusEndpoint(
	ctx context.Context,
	request *operatorservice.CreateNexusEndpointRequest,
	opts ...grpc.CallOption,
) (*operatorservice.CreateNexusEndpointResponse, error) {
	var resp *operatorservice.CreateNexusEndpointResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.CreateNexusEndpoint(ctx, request, opts...)
}

func (c *lazyClient) DeleteNamespace(
	ctx context.Context,
	request *operatorservice.DeleteNamespaceRequest,
	opts ...grpc.CallOption,
) (*operatorservice.DeleteNamespaceResponse, error) {
	var resp *operatorservice.DeleteNamespaceResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.DeleteNamespace(ctx, request, opts...)
}

func (c *lazyClient) DeleteNexusEndpoint(
	ctx context.Context,
	request *operatorservice.DeleteNexusEndpointRequest,
	opts ...grpc.CallOption,
) (*operatorservice.DeleteNexusEndpointResponse, error) {
	var resp *operatorservice.DeleteNexusEndpointResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.DeleteNexusEndpoint(ctx, request, opts...)
}

func (c *lazyClient) GetNexusEndpoint(
	ctx context.Context,
	request *operatorservice.GetNexusEndpointRequest,
	opts ...grpc.CallOption,
) (*operatorservice.GetNexusEndpointResponse, error) {
	var resp *operatorservice.GetNexusEndpointResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.GetNexusEndpoint(ctx, request, opts...)
}

func (c *lazyClient) ListClusters(
	ctx context.Context,
	request *operatorservice.ListClustersRequest,
	opts ...grpc.CallOption,
) (*operatorservice.ListClustersResponse, error) {
	var resp *operatorservice.ListClustersResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.ListClusters(ctx, request, opts...)
}

func (c *lazyClient) ListNexusEndpoints(
	ctx context.Context,
	request *operatorservice.ListNexusEndpointsRequest,
	opts ...grpc.CallOption,
) (*operatorservice.ListNexusEndpointsResponse, error) {
	var resp *operatorservice.ListNexusEndpointsResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.ListNexusEndpoints(ctx, request, opts...)
}

func (c *lazyClient) ListSearchAttributes(
	ctx context.Context,
	request *operatorservice.ListSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*operatorservice.ListSearchAttributesResponse, error) {
	var resp *operatorservice.ListSearchAttributesResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.ListSearchAttributes(ctx, request, opts...)
}

func (c *lazyClient) RemoveRemoteCluster(
	ctx context.Context,
	request *operatorservice.RemoveRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*operatorservice.RemoveRemoteClusterResponse, error) {
	var resp *operatorservice.RemoveRemoteClusterResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.RemoveRemoteCluster(ctx, request, opts...)
}

func (c *lazyClient) RemoveSearchAttributes(
	ctx context.Context,
	request *operatorservice.RemoveSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*operatorservice.RemoveSearchAttributesResponse, error) {
	var resp *operatorservice.RemoveSearchAttributesResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.RemoveSearchAttributes(ctx, request, opts...)
}

func (c *lazyClient) UpdateNexusEndpoint(
	ctx context.Context,
	request *operatorservice.UpdateNexusEndpointRequest,
	opts ...grpc.CallOption,
) (*operatorservice.UpdateNexusEndpointResponse, error) {
	var resp *operatorservice.UpdateNexusEndpointResponse
	client, err := c.clientProvider.GetOperatorClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateNexusEndpoint(ctx, request, opts...)
}
