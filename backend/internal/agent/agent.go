package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/dockpulse/dockmgr/internal/driver"
	"github.com/gorilla/websocket"
)

type Config struct {
	ServerURL string
	Token     string
	HostID    string
	BaseDir   string
}

type Agent struct {
	cfg     Config
	driver  *driver.SocketDriver
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func NewAgent(cfg Config) (*Agent, error) {
	d, err := driver.NewSocketDriver("local")
	if err != nil {
		return nil, fmt.Errorf("failed to init local docker driver: %w", err)
	}

	return &Agent{
		cfg:    cfg,
		driver: d,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Printf("[DockPulse Agent] Connecting to server %s...", a.cfg.ServerURL)
			if err := a.connectAndListen(ctx); err != nil {
				log.Printf("[DockPulse Agent] Connection error: %v. Retrying in 5s...", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (a *Agent) connectAndListen(ctx context.Context) error {
	u, err := url.Parse(a.cfg.ServerURL)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("token", a.cfg.Token)
	q.Set("host_id", a.cfg.HostID)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	a.conn = conn
	log.Printf("[DockPulse Agent] Connected successfully. Registered HostID: %s", a.cfg.HostID)

	for {
		var msg driver.AgentMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}

		go a.handleMessage(ctx, msg)
	}
}

func (a *Agent) sendReply(msg driver.AgentMessage) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	if a.conn != nil {
		_ = a.conn.WriteJSON(msg)
	}
}

func (a *Agent) handleMessage(ctx context.Context, msg driver.AgentMessage) {
	reply := driver.AgentMessage{
		ID: msg.ID,
	}

	switch msg.Action {
	case "ping":
		reply.Payload = json.RawMessage(`{"pong": true}`)
		a.sendReply(reply)

	case "system_info":
		info, err := a.driver.GetSystemInfo(ctx)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(info)
		}
		a.sendReply(reply)

	case "list_containers":
		list, err := a.driver.ListContainers(ctx)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(list)
		}
		a.sendReply(reply)

	case "start_container":
		var p struct{ ID string }
		_ = json.Unmarshal(msg.Payload, &p)
		if err := a.driver.StartContainer(ctx, p.ID); err != nil {
			reply.Error = err.Error()
		}
		a.sendReply(reply)

	case "stop_container":
		var p struct{ ID string }
		_ = json.Unmarshal(msg.Payload, &p)
		if err := a.driver.StopContainer(ctx, p.ID); err != nil {
			reply.Error = err.Error()
		}
		a.sendReply(reply)

	case "restart_container":
		var p struct{ ID string }
		_ = json.Unmarshal(msg.Payload, &p)
		if err := a.driver.RestartContainer(ctx, p.ID); err != nil {
			reply.Error = err.Error()
		}
		a.sendReply(reply)

	case "remove_container":
		var p struct {
			ID    string
			Force bool
		}
		_ = json.Unmarshal(msg.Payload, &p)
		if err := a.driver.RemoveContainer(ctx, p.ID, p.Force); err != nil {
			reply.Error = err.Error()
		}
		a.sendReply(reply)

	case "stream_logs":
		var p struct {
			ContainerID string `json:"container_id"`
			Follow      bool   `json:"follow"`
			Tail        string `json:"tail"`
		}
		_ = json.Unmarshal(msg.Payload, &p)

		writer := &wsStreamWriter{
			agent: a,
			reqID: msg.ID,
		}
		err := a.driver.StreamLogs(ctx, p.ContainerID, p.Follow, p.Tail, writer)
		if err != nil {
			a.sendReply(driver.AgentMessage{ID: msg.ID, Error: err.Error(), Done: true})
		} else {
			a.sendReply(driver.AgentMessage{ID: msg.ID, Done: true})
		}

	case "discover_stacks":
		var p struct{ BaseDir string }
		_ = json.Unmarshal(msg.Payload, &p)
		base := p.BaseDir
		if base == "" {
			base = a.cfg.BaseDir
		}
		stacks, err := a.driver.DiscoverStacks(ctx, base)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(stacks)
		}
		a.sendReply(reply)

	case "read_stack_files":
		var p struct{ Path string }
		_ = json.Unmarshal(msg.Payload, &p)
		c, e, err := a.driver.ReadStackFiles(ctx, p.Path)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(map[string]string{"compose": c, "env": e})
		}
		a.sendReply(reply)

	case "write_stack_files":
		var p struct {
			Path    string `json:"path"`
			Compose string `json:"compose"`
			Env     string `json:"env"`
		}
		_ = json.Unmarshal(msg.Payload, &p)
		if err := a.driver.WriteStackFiles(ctx, p.Path, p.Compose, p.Env); err != nil {
			reply.Error = err.Error()
		}
		a.sendReply(reply)

	case "execute_compose":
		var p struct {
			Path   string `json:"path"`
			Action string `json:"action"`
		}
		_ = json.Unmarshal(msg.Payload, &p)

		writer := &wsStreamWriter{
			agent: a,
			reqID: msg.ID,
		}
		err := a.driver.ExecuteCompose(ctx, p.Path, p.Action, writer)
		if err != nil {
			a.sendReply(driver.AgentMessage{ID: msg.ID, Error: err.Error(), Done: true})
		} else {
			a.sendReply(driver.AgentMessage{ID: msg.ID, Done: true})
		}

	case "list_networks":
		nets, err := a.driver.ListNetworks(ctx)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(nets)
		}
		a.sendReply(reply)

	case "disk_usage":
		du, err := a.driver.GetDiskUsage(ctx)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(du)
		}
		a.sendReply(reply)

	case "prune_resources":
		var p struct{ PruneAll bool }
		_ = json.Unmarshal(msg.Payload, &p)
		rep, err := a.driver.PruneResources(ctx, p.PruneAll)
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.Payload, _ = json.Marshal(rep)
		}
		a.sendReply(reply)
	}
}

type wsStreamWriter struct {
	agent *Agent
	reqID string
}

func (w *wsStreamWriter) Write(p []byte) (n int, err error) {
	buf := make([]byte, len(p))
	copy(buf, p)
	w.agent.sendReply(driver.AgentMessage{
		ID:        w.reqID,
		DataChunk: buf,
		Stream:    true,
	})
	return len(p), nil
}
