package transport

// func testTransportWithCfg(t *testing.T, muxClientCfg config.MuxTransportConfig, muxServerCfg config.MuxTransportConfig) {
// 	muxClientManager := NewTransportManager(config.NewMockConfigProvider(
// 		config.S2SProxyConfig{
// 			MuxTransports: []config.MuxTransportConfig{muxClientCfg},
// 		}), testLogger)

// 	muxServerManager := NewTransportManager(config.NewMockConfigProvider(
// 		config.S2SProxyConfig{
// 			MuxTransports: []config.MuxTransportConfig{muxServerCfg},
// 		}), testLogger)

// 	t.Run("client call server", func(t *testing.T) {
// 		testMuxConnection(t, muxClientManager, muxServerManager)
// 	})

//		t.Run("server call client", func(t *testing.T) {
//			testMuxConnection(t, muxServerManager, muxClientManager)
//		})
//	}

// func TestMuxTransportClose(t *testing.T) {
// 	muxClientManager := NewTransportManager(config.NewMockConfigProvider(
// 		config.S2SProxyConfig{
// 			MuxTransports: []config.MuxTransportConfig{testMuxClientCfg},
// 		}), logger)

// 	muxServerManager := NewTransportManager(config.NewMockConfigProvider(
// 		config.S2SProxyConfig{
// 			MuxTransports: []config.MuxTransportConfig{testMuxServerCfg},
// 		}), logger)

// 	testClose := func(t *testing.T, close TransportManager, wait TransportManager) {
// 		closeTs, waitTs := connect(t, close, wait)
// 		closeTs.Close()
// 		require.True(t, closeTs.IsClosed())

// 		<-waitTs.CloseChan()
// 		waitTs.Close() // call Close to make sure all underlying connection is closed
// 		require.True(t, waitTs.IsClosed())
// 	}

// 	t.Run("close server", func(t *testing.T) {
// 		testClose(t, muxClientManager, muxServerManager)
// 	})

// 	time.Sleep(time.Second)

// 	t.Run("close client", func(t *testing.T) {
// 		testClose(t, muxServerManager, muxClientManager)
// 	})
// }
