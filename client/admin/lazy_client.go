package admin

//type (
//	lazyClient struct {
//		clientProvider client.ClientProvider
//	}
//)
//
//func NewLazyClient(
//	clientProvider client.ClientProvider,
//) adminservice.AdminServiceClient {
//	return &lazyClient{
//		clientProvider: clientProvider,
//	}
//}
//
//// rpcwrappers (https://github.com/temporalio/temporal/blob/main/cmd/tools/rpcwrappers/main.go) doesn't
//// support gRPC stream API. Add it manually.
//func (c *lazyClient) StreamWorkflowReplicationMessages(
//	ctx context.Context,
//	opts ...grpc.CallOption,
//) (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
//	var resp adminservice.AdminService_StreamWorkflowReplicationMessagesClient
//	client, err := c.clientProvider.GetAdminClient()
//	if err != nil {
//		return resp, err
//	}
//
//	return client.StreamWorkflowReplicationMessages(ctx, opts...)
//}
