package supervisor

func (s Server) statusWG() Response {
	return s.runCommand(ActionWGStatus, "wg", "show")
}
