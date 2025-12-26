package config

import (
	"fmt"

	"github.com/temporalio/s2s-proxy/encryption"
)

// ToClusterConnConfig converts from previous versions of proxy config to the new format without requiring a rewrite.
func ToClusterConnConfig(config S2SProxyConfig) S2SProxyConfig {
	if len(config.ClusterConnections) != 0 {
		return config
	}
	return S2SProxyConfig{
		ClusterConnections: []ClusterConnConfig{
			{
				Name: fmt.Sprintf("%s/%s", config.Inbound.Name, config.Outbound.Name),
				LocalServer: ClusterDefinition{
					Connection: TransportInfo{
						ConnectionType: determineConnectionType(config, true),
						TcpClient:      translateClientTCPTLSInfo(config.Inbound.Client.TCPClientSetting),
						TcpServer:      translateServerTCPTLSInfo(config.Outbound.Server.TCPServerSetting),
						MuxCount:       getMuxConnectionCount(config, config.Inbound.Client.MuxTransportName),
						MuxAddressInfo: getMuxAddressInfo(config, config.Inbound.Client.MuxTransportName),
					},
					ACLPolicy:    config.Inbound.ACLPolicy,
					APIOverrides: config.Inbound.APIOverrides,
				},
				RemoteServer: ClusterDefinition{
					Connection: TransportInfo{
						ConnectionType: determineConnectionType(config, false),
						TcpClient:      translateClientTCPTLSInfo(config.Outbound.Client.TCPClientSetting),
						TcpServer:      translateServerTCPTLSInfo(config.Inbound.Server.TCPServerSetting),
						MuxCount:       getMuxConnectionCount(config, config.Outbound.Client.MuxTransportName),
						MuxAddressInfo: getMuxAddressInfo(config, config.Outbound.Client.MuxTransportName),
					},
					ACLPolicy:    config.Outbound.ACLPolicy,
					APIOverrides: config.Outbound.APIOverrides,
				},
				NamespaceTranslation:       nsTranslationToStringTranslator(config.NamespaceNameTranslation),
				SearchAttributeTranslation: config.SearchAttributeTranslation,
				OutboundHealthCheck:        flattenNilHealthCheck(config.OutboundHealthCheck),
				InboundHealthCheck:         flattenNilHealthCheck(config.HealthCheck),
				ShardCountConfig:           config.ShardCountConfig,
				MemberlistConfig:           config.MemberlistConfig,
			},
		},
		Metrics:         config.Metrics,
		ProfilingConfig: config.ProfilingConfig,
		Logging:         config.Logging,
	}
}

func flattenNilHealthCheck(config *HealthCheckConfig) HealthCheckConfig {
	if config == nil {
		return HealthCheckConfig{}
	} else {
		return *config
	}
}

func nsTranslationToStringTranslator(nsTranslation NameTranslationConfig) StringTranslator {
	stringTranslator := StringTranslator{
		Mappings: make([]StringMapping, len(nsTranslation.Mappings)),
	}
	for i, mapping := range nsTranslation.Mappings {
		stringTranslator.Mappings[i] = StringMapping{mapping.LocalName, mapping.RemoteName}
	}
	return stringTranslator
}

func getMuxAddressInfo(config S2SProxyConfig, muxName string) TCPTLSInfo {
	mux := findTransport(config.MuxTransports, muxName)
	if mux.Mode == ServerMode {
		return translateServerTCPTLSInfo(mux.Server)
	} else {
		return translateClientTCPTLSInfo(mux.Client)
	}
}

func getMuxConnectionCount(config S2SProxyConfig, muxName string) int {
	return findTransport(config.MuxTransports, muxName).NumConnections
}

func determineConnectionType(proxyCfg S2SProxyConfig, isLocal bool) ConnectionType {
	source := proxyCfg.Inbound
	if !isLocal {
		source = proxyCfg.Outbound
	}
	switch source.Client.Type {
	case TCPTransport:
		return ConnTypeTCP
	case MuxTransport:
		mode := findTransport(proxyCfg.MuxTransports, source.Client.MuxTransportName).Mode
		switch mode {
		case ServerMode:
			return ConnTypeMuxServer
		case ClientMode:
			return ConnTypeMuxClient
		default:
			// Panic is ok here because the legacy config can only ever have one connection. If this is misconfigured,
			// the whole proxy won't work anyway.
			panic(fmt.Sprintf("couldn't find mux transport \"%s\" in %v", source.Client.MuxTransportName, proxyCfg.MuxTransports))
		}
	default:
		return ConnTypeTCP
	}
}
func findTransport(muxes []MuxTransportConfig, name string) MuxTransportConfig {
	for _, m := range muxes {
		if m.Name == name {
			return m
		}
	}
	return MuxTransportConfig{}
}

func translateClientTCPTLSInfo(cfg TCPClientSetting) TCPTLSInfo {
	return TCPTLSInfo{
		ConnectionString: cfg.ServerAddress,
		TLSConfig: encryption.TLSConfig{
			CertificatePath: cfg.TLS.CertificatePath,
			KeyPath:         cfg.TLS.KeyPath,
			RemoteCAPath:    cfg.TLS.ServerCAPath,
			CAServerName:    cfg.TLS.ServerName,
			VerifyCA:        cfg.TLS.ServerName != "" && cfg.TLS.ServerCAPath != "",
		},
	}
}
func translateServerTCPTLSInfo(cfg TCPServerSetting) TCPTLSInfo {
	return TCPTLSInfo{
		ConnectionString: cfg.ListenAddress,
		TLSConfig: encryption.TLSConfig{
			CertificatePath: cfg.TLS.CertificatePath,
			KeyPath:         cfg.TLS.KeyPath,
			RemoteCAPath:    cfg.TLS.ClientCAPath,
			VerifyCA:        cfg.TLS.RequireClientAuth,
		},
	}
}
