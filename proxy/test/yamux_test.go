package proxy

import (
	"context"
	"log"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serverAddr   = "localhost:50051"
	serverListen = ":50051"
)

// Implement a simple gRPC service
type ClientService struct {
	adminservice.UnimplementedAdminServiceServer
}

func (s *ClientService) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return &adminservice.DescribeClusterResponse{
		ClusterName: "abc",
	}, nil
}

func sessionRead(session *yamux.Session) {
	stream, err := session.Accept()
	if err != nil {
		panic(err)
	}

	buf := make([]byte, 4)
	n, err := stream.Read(buf)
	log.Printf("[stream.Read] Read n:%d, err: %v\n", n, err)
}

func sessionWrite(session *yamux.Session) {
	stream, err := session.Open()
	if err != nil {
		panic(err)
	}

	log.Printf("[stream.Write] write\n")
	stream.Write([]byte("ping"))
}

func yamuxClient() {
	// Get a TCP connection
	log.Printf("[client] Connecting to server at %s...", serverAddr)
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		panic(err)
	}

	// Setup client side of yamux
	session, err := yamux.Client(conn, nil)
	if err != nil {
		panic(err)
	}

	grpcServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcServer, &ClientService{})

	log.Println("[Client] gRPC server listening on session")
	if err := grpcServer.Serve(session); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("[Client] stop")
}

func customDialer(conn net.Conn) (*grpc.ClientConn, error) {
	// Create a gRPC dialer using the existing connection
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return conn, nil // Return the original TCP connection
	}

	// Establish a gRPC client connection using the custom dialer
	return grpc.Dial(
		"unused", // Address is ignored since we're using a custom dialer
		grpc.WithTransportCredentials(insecure.NewCredentials()), // No TLS for simplicity
		grpc.WithContextDialer(dialer),
	)
}

func yamuxServer() {
	// Simulate an incoming client TCP connection
	listener, err := net.Listen("tcp", serverListen)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Println("[Server] listening on ", serverListen)

	// Accept a TCP connection
	conn, err := listener.Accept()
	if err != nil {
		panic(err)
	}

	// Setup server side of yamux
	session, err := yamux.Server(conn, nil)
	if err != nil {
		panic(err)
	}

	stream, err := session.Open()
	if err != nil {
		panic(err)
	}

	// Create a gRPC client using the accepted connection
	grpcClientConn, err := customDialer(stream)
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer grpcClientConn.Close()

	log.Println("[Server] create client")
	client := adminservice.NewAdminServiceClient(grpcClientConn)

	// Perform a unary RPC call
	log.Println("[Server] DescribeCluster")
	req := &adminservice.DescribeClusterRequest{}
	res, err := client.DescribeCluster(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed unary RPC: %v", err)
	}
	log.Printf("[Server] Received response: %v\n", res)

}

func TestYamux(t *testing.T) {
	go yamuxClient()
	go yamuxServer()

	time.Sleep(2 * time.Second)
}
