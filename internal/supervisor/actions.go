package supervisor

import "fmt"

func (s Server) dispatch(req Request) Response {
	switch req.Action {
	case ActionPing:
		return Response{OK: true, Action: req.Action, Output: "pong"}
	case ActionBIRDStart:
		return s.startBIRD()
	case ActionBIRDStop:
		return s.stopBIRD()
	case ActionBIRDReload:
		return s.reloadBIRD()
	case ActionBIRDStatus:
		return s.statusBIRD()
	case ActionWGStart:
		return s.startWG()
	case ActionWGStop:
		return s.stopWG()
	case ActionWGRestart:
		return s.restartWG()
	case ActionWGStatus:
		return s.statusWG()
	case ActionRoutesApply:
		return s.applyRoutes()
	default:
		return Response{OK: false, Action: req.Action, Error: fmt.Sprintf("unknown supervisor action %q", req.Action)}
	}
}
