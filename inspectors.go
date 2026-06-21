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
}
