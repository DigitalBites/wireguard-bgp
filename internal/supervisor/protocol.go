package supervisor

import "time"

const (
	DefaultSocketPath     = "/run/peplink-wg-bgp/supervisor.sock"
	DefaultBIRDConfigPath = "/app-state/bird/bird.conf"
	DefaultBIRDSocketPath = "/run/bird/bird.ctl"
	ActionPing            = "ping"
	ActionBIRDStart       = "bird.start"
	ActionBIRDReload      = "bird.reload"
	ActionBIRDStatus      = "bird.status"
	ActionWGStart         = "wg.start"
	ActionWGStop          = "wg.stop"
	ActionWGRestart       = "wg.restart"
	ActionWGStatus        = "wg.status"
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
	BIRDConfigPath string
	BIRDSocketPath string
}

type Client struct {
	SocketPath string
	Timeout    time.Duration
}
