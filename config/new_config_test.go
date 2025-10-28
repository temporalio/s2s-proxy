package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type Tuple[K, V any] struct {
	k K
	v V
}

func NewTuple[K, V any](k K, v V) Tuple[K, V] {
	return Tuple[K, V]{k: k, v: v}
}

func TestBasic(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "sample-cluster-conn-config.yaml")

	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	require.Equal(t, 1, len(proxyConfig.ClusterConnections))
	require.Equal(t, "127.0.0.1:911", proxyConfig.ClusterConnections[0].OutboundHealthCheck.ListenAddress)
	require.Equal(t, "127.0.0.1:912", proxyConfig.ClusterConnections[0].InboundHealthCheck.ListenAddress)
	require.Equal(t, "myCoolCluster", proxyConfig.ClusterConnections[0].Name)
	require.Equal(t, ConnectionType("mux-server"), proxyConfig.ClusterConnections[0].RemoteServer.Connection.ConnectionType)
	require.Equal(t, 10, proxyConfig.ClusterConnections[0].RemoteServer.Connection.MuxCount)
	require.Equal(t, "127.0.0.1:9004", proxyConfig.ClusterConnections[0].RemoteServer.Connection.MuxAddressInfo.ConnectionString)
	require.Equal(t, "", proxyConfig.ClusterConnections[0].RemoteServer.Connection.TcpServer.ConnectionString)
	require.Equal(t, "", proxyConfig.ClusterConnections[0].RemoteServer.Connection.TcpClient.ConnectionString)
	require.Equal(t, false, proxyConfig.ClusterConnections[0].RemoteServer.Connection.MuxAddressInfo.TLSConfig.ValidateClientCA)
	nsTranslation, err := proxyConfig.ClusterConnections[0].NamespaceTranslation.AsLocalToRemoteBiMap()
	require.NoError(t, err)
	require.Equal(t, "remoteName", nsTranslation.Get("localName"))
	require.Equal(t, "localName", nsTranslation.Inverse().Get("remoteName"))
	require.Equal(t, "", nsTranslation.Get("UnknownName"))
	require.Equal(t, "", nsTranslation.Inverse().Get("UnknownName"))
	require.Equal(t, NewTuple("", false), NewTuple(nsTranslation.GetExists("UnknownName")))
	require.Equal(t, NewTuple("", false), NewTuple(nsTranslation.Inverse().GetExists("UnknownName")))
	saTranslation, err := proxyConfig.ClusterConnections[0].SearchAttributeTranslation.AsLocalToRemoteSATranslation()
	require.NoError(t, err)
	require.Equal(t, "remoteSearchAttribute", saTranslation.Get("namespace-id-1", "localSearchAttribute"))
	require.Equal(t, "localSearchAttribute", saTranslation.Inverse().Get("namespace-id-1", "remoteSearchAttribute"))
}

func TestConversion(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "sample-config.yaml")

	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	converted := ToClusterConnConfig(proxyConfig)
	require.Equal(t, 1, len(converted.ClusterConnections))
	require.Nil(t, converted.Inbound)
	require.Nil(t, converted.Outbound)
	require.Equal(t, ConnTypeTCP, converted.ClusterConnections[0].RemoteServer.Connection.ConnectionType)
	require.True(t, converted.ClusterConnections[0].RemoteServer.Connection.TcpServer.TLSConfig.ValidateClientCA)
	require.Equal(t, ConnTypeTCP, converted.ClusterConnections[0].LocalServer.Connection.ConnectionType)
	require.Equal(t, "AddOrUpdateRemoteCluster", converted.ClusterConnections[0].LocalServer.ACLPolicy.AllowedMethods.AdminService[0])
	require.Equal(t, "namespace1", converted.ClusterConnections[0].LocalServer.ACLPolicy.AllowedNamespaces[0])
	require.Equal(t, int64(100), *converted.ClusterConnections[0].LocalServer.APIOverrides.AdminSerivce.DescribeCluster.Response.FailoverVersionIncrement)
}
