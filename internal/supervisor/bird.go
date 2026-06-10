package supervisor

func (s Server) startBIRD() Response {
	return s.runCommand(ActionBIRDStart, "bird", "-c", s.birdConfigPath(), "-s", s.birdSocketPath())
}

func (s Server) reloadBIRD() Response {
	return s.runCommand(ActionBIRDReload, "birdc", "-s", s.birdSocketPath(), "configure")
}

func (s Server) statusBIRD() Response {
	return s.runCommand(ActionBIRDStatus, "birdc", "-s", s.birdSocketPath(), "show", "protocols")
}

func (s Server) birdConfigPath() string {
	if s.BIRDConfigPath == "" {
		return DefaultBIRDConfigPath
	}
	return s.BIRDConfigPath
}

func (s Server) birdSocketPath() string {
	if s.BIRDSocketPath == "" {
		return DefaultBIRDSocketPath
	}
	return s.BIRDSocketPath
}
