package supervisor

import "time"

const (
	DefaultSocketPath     = "/run/peplink-wg-bgp/supervisor.sock"
	DefaultAppConfigPath  = "/app-state/app.yaml"
	DefaultBIRDConfigPath = "/app-state/bird/bird.conf"
	DefaultBIRDSocketPath = "/run/bird/bird.ctl"
	ActionPing            = "ping"
	ActionBIRDStart       = "bird.start"
	ActionBIRDStop        = "bird.stop"
	ActionBIRDReload      = "bird.reload"
	ActionBIRDStatus      = "bird.status"
	ActionWGStart         = "wg.start"
	ActionWGStop          = "wg.stop"
	ActionWGRestart       = "wg.restart"
	ActionWGStatus        = "wg.status"
	ActionRoutesApply     = "routes.apply"
)

type Request struct {
	Action string `json:"action"`
}

type Response struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Server struct {
	SocketPath     string
	AllowedUID     int
	CommandTimeout time.Duration
	Runner         CommandRunner
	WGManager      WGManager
	RouteManager   RouteManager
	AppConfigPath  string
	BIRDConfigPath string
	BIRDSocketPath string
}

type Client struct {
	SocketPath string
	Timeout    time.Duration
}
