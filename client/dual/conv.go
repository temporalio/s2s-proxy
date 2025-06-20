package dual

import (
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
)

// AddOrUpdateRemoteCluster converter functions
func convertAddOrUpdateRemoteClusterRequest(request *adminservice.AddOrUpdateRemoteClusterRequest) *operatorservice.AddOrUpdateRemoteClusterRequest {
	return &operatorservice.AddOrUpdateRemoteClusterRequest{
		FrontendAddress:               request.GetFrontendAddress(),
		FrontendHttpAddress:           request.GetFrontendHttpAddress(), // Deprecated, used for backward compatibility
		EnableRemoteClusterConnection: request.GetEnableRemoteClusterConnection(),
	}
}

func convertAddOrUpdateRemoteClusterResponse(_ *operatorservice.AddOrUpdateRemoteClusterResponse) *adminservice.AddOrUpdateRemoteClusterResponse {
	return &adminservice.AddOrUpdateRemoteClusterResponse{}
}

// AddSearchAttributes converter functions
func convertAddSearchAttributesRequest(request *adminservice.AddSearchAttributesRequest) *operatorservice.AddSearchAttributesRequest {
	return &operatorservice.AddSearchAttributesRequest{
		Namespace:        request.GetNamespace(),
		SearchAttributes: request.GetSearchAttributes(),
		// operatorService dropped support for IndexName, so we do nothing with it.
		// operatorService dropped support for SkipSchemaUpdate, so we do nothing with it also.
	}
}

func convertAddSearchAttributesResponse(_ *operatorservice.AddSearchAttributesResponse) *adminservice.AddSearchAttributesResponse {
	return &adminservice.AddSearchAttributesResponse{}
}

// GetSearchAttributes converter functions
func convertGetSearchAttributesRequest(request *adminservice.GetSearchAttributesRequest) *operatorservice.ListSearchAttributesRequest {
	return &operatorservice.ListSearchAttributesRequest{
		Namespace: request.GetNamespace(),
		// operatorService dropped support for IndexName, so we do nothing with it.
	}
}

func convertGetSearchAttributesResponse(operatorResponse *operatorservice.ListSearchAttributesResponse) *adminservice.GetSearchAttributesResponse {
	adminResponse := &adminservice.GetSearchAttributesResponse{
		SystemAttributes: operatorResponse.GetSystemAttributes(),
		CustomAttributes: operatorResponse.GetCustomAttributes(),
		Mapping:          operatorResponse.GetStorageSchema(),
	}
	return adminResponse
}

// RemoveSearchAttributes converter functions
func convertRemoveSearchAttributesRequest(request *adminservice.RemoveSearchAttributesRequest) *operatorservice.RemoveSearchAttributesRequest {
	return &operatorservice.RemoveSearchAttributesRequest{
		Namespace:        request.GetNamespace(),
		SearchAttributes: request.GetSearchAttributes(),
		// operatorService dropped support for IndexName, so we do nothing with it.
	}
}

func convertRemoveSearchAttributesResponse(_ *operatorservice.RemoveSearchAttributesResponse) *adminservice.RemoveSearchAttributesResponse {
	return &adminservice.RemoveSearchAttributesResponse{}
}

// ListClusters converter functions
func convertListClustersRequest(request *adminservice.ListClustersRequest) *operatorservice.ListClustersRequest {
	return &operatorservice.ListClustersRequest{
		PageSize:      request.GetPageSize(),
		NextPageToken: request.GetNextPageToken(),
	}
}

func convertListClustersResponse(operatorResponse *operatorservice.ListClustersResponse) *adminservice.ListClustersResponse {
	adminResponse := &adminservice.ListClustersResponse{
		NextPageToken: operatorResponse.GetNextPageToken(),
	}
	for _, cluster := range operatorResponse.GetClusters() {
		adminResponse.Clusters = append(adminResponse.Clusters, &persistence.ClusterMetadata{
			ClusterName:            cluster.GetClusterName(),
			ClusterId:              cluster.GetClusterId(),
			ClusterAddress:         cluster.GetAddress(),
			HttpAddress:            cluster.GetHttpAddress(),
			InitialFailoverVersion: cluster.GetInitialFailoverVersion(),
			HistoryShardCount:      cluster.GetHistoryShardCount(),
			IsConnectionEnabled:    cluster.GetIsConnectionEnabled(),
			// There are more fields in the response, but we can only support the ones that the operator service offers
		})
	}
	return adminResponse
}

// RemoveRemoteCluster converter functions
func convertRemoveRemoteClusterRequest(request *adminservice.RemoveRemoteClusterRequest) *operatorservice.RemoveRemoteClusterRequest {
	return &operatorservice.RemoveRemoteClusterRequest{
		ClusterName: request.GetClusterName(),
	}
}

func convertRemoveRemoteClusterResponse(_ *operatorservice.RemoveRemoteClusterResponse) *adminservice.RemoveRemoteClusterResponse {
	// RemoveRemoteCluster returns an empty response
	return &adminservice.RemoveRemoteClusterResponse{}
}
