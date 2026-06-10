package server

import (
	"net"
	"net/http"

	"peplink-wg-bgp/internal/orchestrator"
	"peplink-wg-bgp/internal/supervisor"
)

func (s *Server) postRoutingApply(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, true, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Apply(r.Context())
	})
}

func (s *Server) postRoutingStart(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, true, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Start(r.Context())
	})
}

func (s *Server) postRoutingStop(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, false, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Stop(r.Context())
	})
}

func (s *Server) postRoutingRestart(w http.ResponseWriter, r *http.Request) {
	s.routingAction(w, r, true, func(routing orchestrator.Routing) orchestrator.Result {
		return routing.Restart(r.Context())
	})
}

func (s *Server) routingAction(
	w http.ResponseWriter,
	r *http.Request,
	pinClient bool,
	run func(orchestrator.Routing) orchestrator.Result,
) {
	if pinClient && s.pinDashboardClientRouteEnabled() {
		if err := s.pinClientRoute(r); err != nil {
			s.logs.Add("error", "client route pin failed", err.Error())
			writeJSONStatus(w, http.StatusServiceUnavailable, orchestrator.Result{
				OK: false,
				Steps: []orchestrator.Step{{
					Action: supervisor.ActionRoutesPinClient,
					OK:     false,
					Error:  err.Error(),
				}},
			})
			return
		}
	}
	result := run(orchestrator.Routing{Client: s.supervisor})
	if !result.OK {
		s.logs.Add("error", "routing action failed", orchestrator.ActionSummary(result))
		writeJSONStatus(w, http.StatusServiceUnavailable, result)
		return
	}
	s.logs.Add("info", "routing action completed", orchestrator.ActionSummary(result))
	writeJSON(w, result)
}

func (s *Server) pinDashboardClientRouteEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Runtime.PinDashboardClientRoute
}

func (s *Server) pinClientRoute(r *http.Request) error {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	_, err = s.supervisor.CallWithParams(r.Context(), supervisor.ActionRoutesPinClient, map[string]string{
		"clientIP": host,
	})
	return err
}
