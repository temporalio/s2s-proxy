package dual

import (
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
)

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

// AddOrUpdateRemoteCluster converter functions
func convertAddOrUpdateRemoteClusterRequest(request *adminservice.AddOrUpdateRemoteClusterRequest) *operatorservice.AddOrUpdateRemoteClusterRequest {
	return &operatorservice.AddOrUpdateRemoteClusterRequest{
		FrontendAddress:               request.GetFrontendAddress(),
		FrontendHttpAddress:           request.GetFrontendHttpAddress(), // Deprecated, used for backward compatibility
		EnableRemoteClusterConnection: request.GetEnableRemoteClusterConnection(),
	}
}

func convertAddOrUpdateRemoteClusterResponse(operatorResponse *operatorservice.AddOrUpdateRemoteClusterResponse) *adminservice.AddOrUpdateRemoteClusterResponse {
	return &adminservice.AddOrUpdateRemoteClusterResponse{}
}

// RemoveRemoteCluster converter functions
func convertRemoveRemoteClusterRequest(request *adminservice.RemoveRemoteClusterRequest) *operatorservice.RemoveRemoteClusterRequest {
	return &operatorservice.RemoveRemoteClusterRequest{
		ClusterName: request.GetClusterName(),
	}
}

func convertRemoveRemoteClusterResponse(operatorResponse *operatorservice.RemoveRemoteClusterResponse) *adminservice.RemoveRemoteClusterResponse {
	// RemoveRemoteCluster returns an empty response
	return &adminservice.RemoveRemoteClusterResponse{}
}
