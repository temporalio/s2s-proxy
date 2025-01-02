package cadencetype

import (
	"context"
	cadence "github.com/uber/cadence-idl/go/proto/admin/v1"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	temporal "go.temporal.io/server/api/adminservice/v1"
)

func GetWorkflowExecutionRawHistoryV2Request(
	ctx context.Context,
	request *temporal.GetWorkflowExecutionRawHistoryV2Request,
	cadenceClient apiv1.DomainAPIYARPCClient,
) *cadence.GetWorkflowExecutionRawHistoryV2Request {
	if request == nil {
		return nil
	}

	resp, err := cadenceClient.DescribeDomain(ctx, &apiv1.DescribeDomainRequest{
		DescribeBy: &apiv1.DescribeDomainRequest_Id{
			Id: request.GetNamespaceId(),
		},
	})

	if err != nil {
		panic("failed to describe domain: " + err.Error())
	}

	return &cadence.GetWorkflowExecutionRawHistoryV2Request{
		Domain:            resp.GetDomain().GetName(),
		WorkflowExecution: WorkflowExecution(request.GetExecution()),
		StartEvent:        &cadence.VersionHistoryItem{EventId: request.GetStartEventId(), Version: request.GetStartEventVersion()},
		EndEvent:          &cadence.VersionHistoryItem{EventId: request.GetEndEventId(), Version: request.GetEndEventVersion()},
		PageSize:          request.GetMaximumPageSize(),
		NextPageToken:     request.GetNextPageToken(),
	}
}

func GetReplicationMessagesRequest(request *temporal.GetReplicationMessagesRequest) *cadence.GetReplicationMessagesRequest {
	if request == nil {
		return nil
	}

	var tokens []*cadence.ReplicationToken
	for _, t := range request.GetTokens() {
		tokens = append(tokens, &cadence.ReplicationToken{
			ShardId:                t.GetShardId() - 1,
			LastRetrievedMessageId: t.GetLastRetrievedMessageId(),
			LastProcessedMessageId: t.GetLastProcessedMessageId(),
		})
	}

	return &cadence.GetReplicationMessagesRequest{
		Tokens:      tokens,
		ClusterName: request.GetClusterName(),
	}
}

func DescribeClusterRequest(in *temporal.DescribeClusterRequest) *cadence.DescribeClusterRequest {
	if in == nil {
		return nil
	}

	return &cadence.DescribeClusterRequest{}
}

func GetNamespaceReplicationMessagesRequest(in *temporal.GetNamespaceReplicationMessagesRequest) *cadence.GetDomainReplicationMessagesRequest {
	if in == nil {
		return nil
	}

	return &cadence.GetDomainReplicationMessagesRequest{
		LastRetrievedMessageId: Int64Value(in.GetLastRetrievedMessageId()),
		LastProcessedMessageId: Int64Value(in.GetLastProcessedMessageId()),
		ClusterName:            in.GetClusterName(),
	}
}
