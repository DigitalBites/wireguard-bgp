package server

import (
	"net/http"

	"peplink-wg-bgp/internal/orchestrator"
)

func (s *Server) postRoutingApply(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Apply(r.Context())
	})
}

func (s *Server) postRoutingStart(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Start(r.Context())
	})
}

func (s *Server) postRoutingStop(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Stop(r.Context())
	})
}

func (s *Server) postRoutingRestart(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Restart(r.Context())
	})
}

func (s *Server) routingAction(
	w http.ResponseWriter,
	r *http.Request,
	run func(orchestrator.Routing) orchestrator.Result,
) {
	result := run(orchestrator.Routing{Client: s.supervisor})
	if !result.OK {
		s.logs.Add("error", "routing action failed", orchestrator.ActionSummary(result))
		writeJSONStatus(w, http.StatusServiceUnavailable, result)
		return
	}
	s.logs.Add("info", "routing action completed", orchestrator.ActionSummary(result))
	writeJSON(w, result)
}
