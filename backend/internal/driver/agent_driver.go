package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type AgentMessage struct {
	ID        string          `json:"id"`
	Action    string          `json:"action"` // "ping", "system_info", "list_containers", "execute_compose", etc.
	Payload   json.RawMessage `json:"payload,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
	Error     string          `json:"error,omitempty"`
	DataChunk []byte          `json:"data_chunk,omitempty"`
	Done      bool            `json:"done,omitempty"`
}

type AgentSession struct {
	HostID     string
	Conn       *websocket.Conn
	mu         sync.Mutex
	pendingMu  sync.RWMutex
	pending    map[string]chan AgentMessage
	streamChan map[string]chan AgentMessage
}

func NewAgentSession(hostID string, conn *websocket.Conn) *AgentSession {
	return &AgentSession{
		HostID:     hostID,
		Conn:       conn,
		pending:    make(map[string]chan AgentMessage),
		streamChan: make(map[string]chan AgentMessage),
	}
}

func (s *AgentSession) StartReceiver() {
	for {
		_, msgBytes, err := s.Conn.ReadMessage()
		if err != nil {
			s.closeAllPending(err)
			return
		}

		var msg AgentMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		s.pendingMu.RLock()
		ch, hasPending := s.pending[msg.ID]
		streamCh, hasStream := s.streamChan[msg.ID]
		s.pendingMu.RUnlock()

		if hasPending {
			ch <- msg
		} else if hasStream {
			streamCh <- msg
		}
	}
}

func (s *AgentSession) closeAllPending(err error) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	for id, ch := range s.streamChan {
		close(ch)
		delete(s.streamChan, id)
	}
}

func (s *AgentSession) Call(ctx context.Context, action string, payload interface{}, result interface{}) error {
	reqID := fmt.Sprintf("%d", time.Now().UnixNano())
	payloadBytes, _ := json.Marshal(payload)

	msg := AgentMessage{
		ID:      reqID,
		Action:  action,
		Payload: payloadBytes,
	}

	respChan := make(chan AgentMessage, 1)
	s.pendingMu.Lock()
	s.pending[reqID] = respChan
	s.pendingMu.Unlock()

	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, reqID)
		s.pendingMu.Unlock()
	}()

	s.mu.Lock()
	err := s.Conn.WriteJSON(msg)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-respChan:
		if !ok {
			return errors.New("agent connection closed")
		}
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		if result != nil && len(resp.Payload) > 0 {
			return json.Unmarshal(resp.Payload, result)
		}
		return nil
	}
}

type AgentManager struct {
	mu       sync.RWMutex
	sessions map[string]*AgentSession
}

func NewAgentManager() *AgentManager {
	return &AgentManager{
		sessions: make(map[string]*AgentSession),
	}
}

func (m *AgentManager) Register(hostID string, conn *websocket.Conn) *AgentSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := NewAgentSession(hostID, conn)
	m.sessions[hostID] = session
	go session.StartReceiver()
	return session
}

func (m *AgentManager) Unregister(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hostID)
}

func (m *AgentManager) GetSession(hostID string) (*AgentSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[hostID]
	if !exists {
		return nil, fmt.Errorf("agent for host %s is not connected", hostID)
	}
	return session, nil
}

// AgentDriver implements HostDriver by wrapping an AgentSession
type AgentDriver struct {
	hostID  string
	manager *AgentManager
}

func NewAgentDriver(hostID string, manager *AgentManager) *AgentDriver {
	return &AgentDriver{
		hostID:  hostID,
		manager: manager,
	}
}

func (d *AgentDriver) getSession() (*AgentSession, error) {
	return d.manager.GetSession(d.hostID)
}

func (d *AgentDriver) Ping(ctx context.Context) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "ping", nil, nil)
}

func (d *AgentDriver) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	session, err := d.getSession()
	if err != nil {
		return nil, err
	}
	var res SystemInfo
	err = session.Call(ctx, "system_info", nil, &res)
	return &res, err
}

func (d *AgentDriver) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	session, err := d.getSession()
	if err != nil {
		return nil, err
	}
	var res []ContainerInfo
	err = session.Call(ctx, "list_containers", nil, &res)
	return res, err
}

func (d *AgentDriver) StartContainer(ctx context.Context, id string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "start_container", map[string]string{"id": id}, nil)
}

func (d *AgentDriver) StopContainer(ctx context.Context, id string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "stop_container", map[string]string{"id": id}, nil)
}

func (d *AgentDriver) RestartContainer(ctx context.Context, id string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "restart_container", map[string]string{"id": id}, nil)
}

func (d *AgentDriver) RemoveContainer(ctx context.Context, id string, force bool) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "remove_container", map[string]interface{}{"id": id, "force": force}, nil)
}

func (d *AgentDriver) StreamLogs(ctx context.Context, containerID string, follow bool, tail string, writer io.Writer) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}

	reqID := fmt.Sprintf("%d", time.Now().UnixNano())
	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"container_id": containerID,
		"follow":       follow,
		"tail":         tail,
	})

	streamCh := make(chan AgentMessage, 100)
	session.pendingMu.Lock()
	session.streamChan[reqID] = streamCh
	session.pendingMu.Unlock()

	defer func() {
		session.pendingMu.Lock()
		delete(session.streamChan, reqID)
		session.pendingMu.Unlock()
	}()

	session.mu.Lock()
	err = session.Conn.WriteJSON(AgentMessage{
		ID:      reqID,
		Action:  "stream_logs",
		Payload: payloadBytes,
		Stream:  true,
	})
	session.mu.Unlock()
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-streamCh:
			if !ok || msg.Done {
				return nil
			}
			if msg.Error != "" {
				return errors.New(msg.Error)
			}
			if len(msg.DataChunk) > 0 {
				writer.Write(msg.DataChunk)
			}
		}
	}
}

func (d *AgentDriver) ExecShell(ctx context.Context, containerID string, cmd []string, in io.Reader, out io.Writer, resizeChan <-chan TerminalSize) error {
	// PTY over WebSocket proxied to Agent
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "exec_shell", map[string]interface{}{"container_id": containerID, "cmd": cmd}, nil)
}

func (d *AgentDriver) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	session, err := d.getSession()
	if err != nil {
		return nil, err
	}
	var res []NetworkInfo
	err = session.Call(ctx, "list_networks", nil, &res)
	return res, err
}

func (d *AgentDriver) CreateNetwork(ctx context.Context, name, driver string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "create_network", map[string]string{"name": name, "driver": driver}, nil)
}

func (d *AgentDriver) RemoveNetwork(ctx context.Context, id string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "remove_network", map[string]string{"id": id}, nil)
}

func (d *AgentDriver) ConnectNetwork(ctx context.Context, netID, containerID string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "connect_network", map[string]string{"net_id": netID, "container_id": containerID}, nil)
}

func (d *AgentDriver) DisconnectNetwork(ctx context.Context, netID, containerID string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "disconnect_network", map[string]string{"net_id": netID, "container_id": containerID}, nil)
}

func (d *AgentDriver) GetDiskUsage(ctx context.Context) (*DiskUsageInfo, error) {
	session, err := d.getSession()
	if err != nil {
		return nil, err
	}
	var res DiskUsageInfo
	err = session.Call(ctx, "disk_usage", nil, &res)
	return &res, err
}

func (d *AgentDriver) PruneResources(ctx context.Context, pruneAll bool) (*PruneReport, error) {
	session, err := d.getSession()
	if err != nil {
		return nil, err
	}
	var res PruneReport
	err = session.Call(ctx, "prune_resources", map[string]bool{"prune_all": pruneAll}, &res)
	return &res, err
}

func (d *AgentDriver) DiscoverStacks(ctx context.Context, baseDir string) ([]DiscoveredStack, error) {
	session, err := d.getSession()
	if err != nil {
		return nil, err
	}
	var res []DiscoveredStack
	err = session.Call(ctx, "discover_stacks", map[string]string{"base_dir": baseDir}, &res)
	return res, err
}

func (d *AgentDriver) ReadStackFiles(ctx context.Context, stackPath string) (string, string, error) {
	session, err := d.getSession()
	if err != nil {
		return "", "", err
	}
	var res struct {
		Compose string `json:"compose"`
		Env     string `json:"env"`
	}
	err = session.Call(ctx, "read_stack_files", map[string]string{"path": stackPath}, &res)
	return res.Compose, res.Env, err
}

func (d *AgentDriver) WriteStackFiles(ctx context.Context, stackPath string, composeContent string, envContent string) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}
	return session.Call(ctx, "write_stack_files", map[string]string{
		"path":    stackPath,
		"compose": composeContent,
		"env":     envContent,
	}, nil)
}

func (d *AgentDriver) ExecuteCompose(ctx context.Context, stackPath string, action string, writer io.Writer) error {
	session, err := d.getSession()
	if err != nil {
		return err
	}

	reqID := fmt.Sprintf("%d", time.Now().UnixNano())
	payloadBytes, _ := json.Marshal(map[string]string{
		"path":   stackPath,
		"action": action,
	})

	streamCh := make(chan AgentMessage, 100)
	session.pendingMu.Lock()
	session.streamChan[reqID] = streamCh
	session.pendingMu.Unlock()

	defer func() {
		session.pendingMu.Lock()
		delete(session.streamChan, reqID)
		session.pendingMu.Unlock()
	}()

	session.mu.Lock()
	err = session.Conn.WriteJSON(AgentMessage{
		ID:      reqID,
		Action:  "execute_compose",
		Payload: payloadBytes,
		Stream:  true,
	})
	session.mu.Unlock()
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-streamCh:
			if !ok || msg.Done {
				return nil
			}
			if msg.Error != "" {
				return errors.New(msg.Error)
			}
			if len(msg.DataChunk) > 0 {
				writer.Write(msg.DataChunk)
			}
		}
	}
}

func (d *AgentDriver) Close() error {
	return nil
}
