package mux

import (
	"context"
	"fmt"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

type ConnListener interface {
	OnConnectionListUpdate(map[string]session.ManagedMuxSession)
}

// NewGRPCMuxManager starts a MuxManager that will start a grpc.Server using the provided definition on every generated mux,
// and will manage the mux connections of the provided grpcutil.MultiClientConn such that they match the list of available muxes
// Caller usage: 1. Create the MultiClientConn, 2. call NewGRPCMuxManager, 3. MultiMuxManager.Start, 4. Use the grpcutil.MultiClientConn
// grpc.Server's will be started on every inbound mux automatically.
func NewGRPCMuxManager(ctx context.Context, name string, cd config.ClusterDefinition, listener ConnListener, serverDefinition *grpc.Server, logger log.Logger) (MultiMuxManager, error) {
	// sane default for existing configs
	muxCount := 10
	if cd.Connection.MuxCount != 0 {
		muxCount = cd.Connection.MuxCount
	}
	connectionTypeName := string(cd.Connection.ConnectionType)
	metricLabels := []string{cd.Connection.MuxAddressInfo.ConnectionString, connectionTypeName, name}
	var muxProviderBuilder MuxProviderBuilder
	logger.Info(fmt.Sprintf("Applying %s mux provider from config: %+v", connectionTypeName, cd))
	switch cd.Connection.ConnectionType {
	case config.ConnTypeMuxClient:
		muxProviderBuilder = func(cb AddNewMux, lifetime context.Context) (MuxProvider, error) {
			return NewMuxEstablisherProvider(lifetime, name, cb, int64(muxCount), cd.Connection.MuxAddressInfo, metricLabels, logger)
		}
	case config.ConnTypeMuxServer:
		muxProviderBuilder = func(cb AddNewMux, lifetime context.Context) (MuxProvider, error) {
			return NewMuxReceiverProvider(lifetime, name, cb, int64(muxCount), cd.Connection.MuxAddressInfo, metricLabels, logger)
		}
	default:
		return nil, fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s", name, connectionTypeName)
	}
	return NewCustomMultiMuxManager(ctx,
		name,
		muxProviderBuilder,
		[]session.StartManagedComponentFn{
			registerYamuxObserverBuilder(name, logger),
			registerGRPCServer(connectionTypeName, serverDefinition, metricLabels, logger),
		},
		[]OnConnectionListUpdate{listener.OnConnectionListUpdate},
		logger,
	)
}

func registerGRPCServer(mode string, serverConfig *grpc.Server, metricLabels []string, logger log.Logger) session.StartManagedComponentFn {
	return func(lifetime context.Context, id string, session *yamux.Session) {
		go func() {
			logger.Info("Starting inbound server for mux", tag.NewStringTag("remote_addr", session.RemoteAddr().String()),
				tag.NewStringTag("mode", mode), tag.NewStringTag("mux_id", id))
			for lifetime.Err() == nil {
				_ = serverConfig.Serve(session)
				metrics.MuxServerDisconnected.WithLabelValues(metricLabels...).Inc()
			}
		}()
	}
}
