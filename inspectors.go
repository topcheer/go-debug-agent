package debugagent

// initInspectors registers all built-in tools.
// Called by NewDebugEngine.
func initInspectors() {
	registerRuntimeInspector()
	registerHTTPTrackerInspector()
	registerSystemInspector()
	registerGoroutineInspector()
	registerDatabaseInspector()
	registerBuildInfoInspector()
	registerAllocInspector()
	registerNetworkInspector()
	registerRedisInspector()
	registerRoutesInspector()
	registerGormInspector()
	registerPprofInspector()

	// v0.4.0 inspectors
	registerLoggingInspector()
	registerCacheInspector()
	registerHttpClientInspector()
	registerFdInspector()
	registerMetricsInspector()
	registerContextInspector()
	registerSyncInspector()

	// v0.5.0 inspectors
	registerSecurityInspector()
	registerHealthInspector()
	registerSchedulerInspector()
	registerErrorInspector()
	registerWebSocketInspector()

	// v0.6.0 inspectors
	registerLocksInspector()
	registerMigrationInspector()
	registerConfigInspector()
	registerFeatureFlagInspector()
	registerEndpointTestInspector()
	registerPoolInspector()
}
