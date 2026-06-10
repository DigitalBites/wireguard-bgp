package supervisor

import (
	"sync"
	"time"
)

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
	ActionBIRDDetails     = "bird.details"
	ActionWGStart         = "wg.start"
	ActionWGStop          = "wg.stop"
	ActionWGRestart       = "wg.restart"
	ActionWGStatus        = "wg.status"
	ActionWGDump          = "wg.dump"
	ActionRoutesApply     = "routes.apply"
	ActionRoutesPinClient = "routes.pin_client"
)

type Request struct {
	Action string            `json:"action"`
	Params map[string]string `json:"params,omitempty"`
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
	BIRDManager    BIRDManager
	ActionLock     *sync.Mutex
	AppConfigPath  string
	BIRDConfigPath string
	BIRDSocketPath string
}

type Client struct {
	SocketPath string
	Timeout    time.Duration
}
