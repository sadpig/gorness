package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gorness.dev/gorness/internal/config"
)

type mcpTransportClient interface {
	Call(ctx context.Context, toolName string, arguments json.RawMessage) (ToolResult, error)
	Close() error
}

// MCPManager 管理 stdio 与 HTTP (含 Streamable HTTP SSE 响应) MCP 服务器连接、工具发现与生命周期回收。
type MCPManager struct {
	secrets       config.SecretResolver
	globalMCPFile string
	workDir       string
	httpClient    *http.Client

	mu      sync.RWMutex
	clients map[string]mcpTransportClient
	tools   []Tool
}

func NewMCPManager(secrets config.SecretResolver, globalMCPFile, workDir string) *MCPManager {
	canonGlobal := ""
	if strings.TrimSpace(globalMCPFile) != "" {
		canonGlobal = canonicalizeExistingOrClean(globalMCPFile)
	}
	return &MCPManager{
		secrets:       secrets,
		globalMCPFile: canonGlobal,
		workDir:       workDir,
		httpClient:    &http.Client{},
		clients:       make(map[string]mcpTransportClient),
	}
}

func (m *MCPManager) ConnectAll(ctx context.Context, cfg config.MCPConfig) error {
	if m == nil || len(cfg.Servers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cfg.Servers))
	for id := range cfg.Servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		srv := cfg.Servers[id]
		if srv.ID == "" {
			srv.ID = id
		}
		if srv.Enabled != nil && !*srv.Enabled {
			continue
		}
		if _, err := m.ConnectServer(ctx, srv); err != nil {
			return fmt.Errorf("connect mcp server %q: %w", srv.ID, err)
		}
	}
	return nil
}

func (m *MCPManager) ConnectServer(ctx context.Context, srv config.MCPServerConfig) ([]Tool, error) {
	if m == nil {
		return nil, errors.New("nil MCPManager")
	}
	srvID := strings.TrimSpace(srv.ID)
	if srvID == "" {
		return nil, errors.New("mcp server id is required")
	}

	// 严格闭合 cmd: 凭据权限：仅当来源文件非空且精确等于全局 ~/.gorness/mcp.toml 时才允许 cmd:。
	allowCmd := false
	if strings.TrimSpace(srv.SourceFile) != "" && m.globalMCPFile != "" {
		allowCmd = canonicalizeExistingOrClean(srv.SourceFile) == m.globalMCPFile
	}

	resolvedEnv, knownSecrets, err := m.resolveKVMap(ctx, srv.Env, allowCmd)
	if err != nil {
		return nil, fmt.Errorf("resolve env for mcp %q: %w", srvID, err)
	}
	resolvedHeaders, hdrSecrets, err := m.resolveKVMap(ctx, srv.Headers, allowCmd)
	if err != nil {
		return nil, fmt.Errorf("resolve headers for mcp %q: %w", srvID, err)
	}
	knownSecrets = append(knownSecrets, hdrSecrets...)

	timeout := 30 * time.Second
	if srv.TimeoutSec > 0 {
		timeout = time.Duration(srv.TimeoutSec) * time.Second
	}

	transport := strings.ToLower(strings.TrimSpace(srv.Transport))
	if transport == "" {
		if strings.TrimSpace(srv.URL) != "" {
			transport = "http"
		} else {
			transport = "stdio"
		}
	}

	var client mcpTransportClient
	var remoteTools []mcpRemoteToolSpec

	switch transport {
	case "stdio":
		sc, toolsList, sErr := startStdioMCPClient(ctx, srv, resolvedEnv, m.workDir, timeout, knownSecrets)
		if sErr != nil {
			return nil, sErr
		}
		client = sc
		remoteTools = toolsList

	case "http", "streamable-http", "sse":
		hc, toolsList, hErr := startHTTPMCPClient(ctx, m.httpClient, srv, resolvedHeaders, timeout, knownSecrets)
		if hErr != nil {
			return nil, hErr
		}
		client = hc
		remoteTools = toolsList

	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", srv.Transport)
	}

	registered := make([]Tool, 0, len(remoteTools))
	for _, rt := range remoteTools {
		remoteName := rt.Name
		fullToolName := formatMCPToolName(srvID, remoteName)
		schema := rt.InputSchema
		if len(bytes.TrimSpace(schema)) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}

		capturedClient := client
		capturedRemote := remoteName
		capturedServerID := srvID
		capturedSecrets := append([]string(nil), knownSecrets...)

		// 安全不变量：所有 MCP 工具强制标记 IsMCP=true, ReadOnly=false, ConcurrencySafe=false，
		// 绝不信任外部服务器自报的 annotations.readOnlyHint 免除 PermissionGate 审查！
		t := Tool{
			Name:            fullToolName,
			Description:     strings.TrimSpace(rt.Description),
			InputSchema:     schema,
			ReadOnly:        false,
			Mutating:        true,
			ConcurrencySafe: false,
			IsMCP:           true,
			MCPServerID:     capturedServerID,
			MCPSourceFile:   srv.SourceFile,
			MCPReadOnlyHint: rt.Annotations.ReadOnlyHint,
			DeferLoad:       true,
			Handler: func(callCtx context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
				res, cErr := capturedClient.Call(callCtx, capturedRemote, rawInput)
				if cErr != nil {
					return ToolResult{}, errors.New(config.Redact(cErr.Error(), append(capturedSecrets, call.Secrets...)))
				}
				allSecrets := append([]string(nil), capturedSecrets...)
				allSecrets = append(allSecrets, call.Secrets...)
				redacted := config.Redact(res.Output, allSecrets)
				res.Output = wrapUntrustedWithDir("mcp:"+capturedServerID+":"+capturedRemote, redacted, logsDirFromVault(call.VaultRoot))
				return res, nil
			},
		}
		registered = append(registered, t)
	}

	m.mu.Lock()
	if old, exists := m.clients[srvID]; exists {
		_ = old.Close()
	}
	m.clients[srvID] = client
	// 替换同名服务器旧工具定义后追加新列表。
	filtered := make([]Tool, 0, len(m.tools)+len(registered))
	for _, existing := range m.tools {
		if existing.MCPServerID != srvID {
			filtered = append(filtered, existing)
		}
	}
	m.tools = append(filtered, registered...)
	m.mu.Unlock()

	return registered, nil
}

func (m *MCPManager) Tools() []Tool {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Tool, len(m.tools))
	copy(out, m.tools)
	return out
}

func (m *MCPManager) RegisterInto(reg *Registry) {
	if m == nil || reg == nil {
		return
	}
	for _, t := range m.Tools() {
		reg.Register(t)
	}
}

func (m *MCPManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	clients := make([]mcpTransportClient, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.clients = make(map[string]mcpTransportClient)
	m.mu.Unlock()

	var firstErr error
	for _, c := range clients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *MCPManager) resolveKVMap(ctx context.Context, in map[string]string, allowCmd bool) (map[string]string, []string, error) {
	if len(in) == 0 {
		return nil, nil, nil
	}
	out := make(map[string]string, len(in))
	var secrets []string
	for k, v := range in {
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "cmd:") && !allowCmd {
			return nil, nil, config.ErrCmdSecretDisallowed
		}
		if m.secrets != nil && (strings.HasPrefix(trimmed, "env:") || strings.HasPrefix(trimmed, "keychain:") || strings.HasPrefix(trimmed, "cmd:")) {
			resolved, err := m.secrets.Resolve(ctx, trimmed, allowCmd)
			if err != nil {
				return nil, nil, err
			}
			out[k] = resolved
			secrets = append(secrets, resolved)
			continue
		}
		out[k] = v
		lk := strings.ToLower(k)
		if strings.Contains(lk, "key") || strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "auth") {
			if trimmed != "" {
				secrets = append(secrets, v)
			}
		}
	}
	return out, secrets, nil
}

type mcpRemoteToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		ReadOnlyHint bool `json:"readOnlyHint"`
	} `json:"annotations"`
}

type mcpToolsListResult struct {
	Tools      []mcpRemoteToolSpec `json:"tools"`
	NextCursor string              `json:"nextCursor"`
}

type mcpCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ============================================================================
// HTTP / Streamable HTTP MCP Client
// ============================================================================

type httpMCPClient struct {
	httpClient *http.Client
	endpoint   string
	headers    map[string]string
	timeout    time.Duration
	secrets    []string

	seq       atomic.Uint64
	sessionMu sync.RWMutex
	sessionID string
}

func startHTTPMCPClient(
	ctx context.Context,
	hc *http.Client,
	srv config.MCPServerConfig,
	headers map[string]string,
	timeout time.Duration,
	secrets []string,
) (*httpMCPClient, []mcpRemoteToolSpec, error) {
	endpoint := strings.TrimSpace(srv.URL)
	if endpoint == "" {
		return nil, nil, errors.New("http mcp server requires url")
	}
	if hc == nil {
		hc = &http.Client{}
	}
	c := &httpMCPClient{
		httpClient: hc,
		endpoint:   endpoint,
		headers:    headers,
		timeout:    timeout,
		secrets:    secrets,
	}

	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gorness", "version": "1.0.0"},
	}
	if _, err := c.rpcRequest(ctx, "initialize", initParams, true); err != nil {
		return nil, nil, err
	}
	_, _ = c.rpcRequest(ctx, "notifications/initialized", map[string]any{}, false)

	var allTools []mcpRemoteToolSpec
	cursor := ""
	for page := 0; page < 20; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		rawRes, err := c.rpcRequest(ctx, "tools/list", params, true)
		if err != nil {
			return nil, nil, err
		}
		var parsed mcpToolsListResult
		if err := json.Unmarshal(rawRes, &parsed); err != nil {
			return nil, nil, fmt.Errorf("decode tools/list: %w", err)
		}
		allTools = append(allTools, parsed.Tools...)
		if parsed.NextCursor == "" || parsed.NextCursor == cursor {
			break
		}
		cursor = parsed.NextCursor
	}
	return c, allTools, nil
}

func (c *httpMCPClient) Call(ctx context.Context, toolName string, arguments json.RawMessage) (ToolResult, error) {
	argsObj := json.RawMessage(`{}`)
	if len(bytes.TrimSpace(arguments)) > 0 {
		argsObj = arguments
	}
	rawRes, err := c.rpcRequest(ctx, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": argsObj,
	}, true)
	if err != nil {
		return ToolResult{}, err
	}
	return decodeMCPCallPayload(rawRes), nil
}

func (c *httpMCPClient) Close() error {
	return nil
}

func (c *httpMCPClient) rpcRequest(ctx context.Context, method string, params any, expectReply bool) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if expectReply {
		payload["id"] = c.seq.Add(1)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	c.sessionMu.RLock()
	sid := c.sessionID
	c.sessionMu.RUnlock()
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.New(config.Redact(err.Error(), c.secrets))
	}
	defer resp.Body.Close()

	if newSID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); newSID != "" {
		c.sessionMu.Lock()
		c.sessionID = newSID
		c.sessionMu.Unlock()
	}

	if resp.StatusCode >= 400 {
		rawErr, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		return nil, fmt.Errorf("mcp HTTP %d: %s", resp.StatusCode, config.Redact(strings.TrimSpace(string(rawErr)), c.secrets))
	}
	if !expectReply || resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	var rpcResp jsonRPCResponse
	if strings.Contains(ct, "text/event-stream") {
		if err := parseSSERPCResponse(rawBody, &rpcResp); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(rawBody, &rpcResp); err != nil {
			return nil, fmt.Errorf("decode mcp json-rpc response: %w", err)
		}
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp rpc error %d: %s", rpcResp.Error.Code, config.Redact(rpcResp.Error.Message, c.secrets))
	}
	return rpcResp.Result, nil
}

func parseSSERPCResponse(raw []byte, out *jsonRPCResponse) error {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				dataLines = dataLines[:0]
				var cand jsonRPCResponse
				if err := json.Unmarshal([]byte(joined), &cand); err == nil && (len(cand.Result) > 0 || cand.Error != nil) {
					*out = cand
					return nil
				}
			}
			continue
		}
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimSpace(after))
		}
	}
	if len(dataLines) > 0 {
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), out); err == nil {
			return nil
		}
	}
	return errors.New("no json-rpc response found in mcp sse stream")
}

// ============================================================================
// Stdio MCP Client
// ============================================================================

type stdioMCPClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	timeout time.Duration
	secrets []string

	writeMu sync.Mutex
	seq     atomic.Uint64

	pendMu  sync.Mutex
	pending map[uint64]chan jsonRPCResponse
	closed  bool
	doneCh  chan struct{}
}

func startStdioMCPClient(
	ctx context.Context,
	srv config.MCPServerConfig,
	extraEnv map[string]string,
	workDir string,
	timeout time.Duration,
	secrets []string,
) (*stdioMCPClient, []mcpRemoteToolSpec, error) {
	command := strings.TrimSpace(srv.Command)
	if command == "" {
		return nil, nil, errors.New("stdio mcp server requires command")
	}

	cmd := exec.Command(command, srv.Args...)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = canonicalizeExistingOrClean(workDir)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	baseEnv := buildCleanCommandEnv(os.TempDir(), nil)
	for k, v := range extraEnv {
		baseEnv = append(baseEnv, k+"="+v)
	}
	cmd.Env = baseEnv

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, err
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("start mcp stdio %q: %w", command, err)
	}

	c := &stdioMCPClient{
		cmd:     cmd,
		stdin:   stdin,
		timeout: timeout,
		secrets: secrets,
		pending: make(map[uint64]chan jsonRPCResponse),
		doneCh:  make(chan struct{}),
	}

	go c.readLoop(stdout)
	go func() {
		_ = cmd.Wait()
		close(c.doneCh)
		c.failAllPending()
	}()

	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gorness", "version": "1.0.0"},
	}
	if _, err := c.rpcRequest(ctx, "initialize", initParams, true); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	_, _ = c.rpcRequest(ctx, "notifications/initialized", map[string]any{}, false)

	var allTools []mcpRemoteToolSpec
	cursor := ""
	for page := 0; page < 20; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		rawRes, err := c.rpcRequest(ctx, "tools/list", params, true)
		if err != nil {
			_ = c.Close()
			return nil, nil, err
		}
		var parsed mcpToolsListResult
		if err := json.Unmarshal(rawRes, &parsed); err != nil {
			_ = c.Close()
			return nil, nil, fmt.Errorf("decode stdio tools/list: %w", err)
		}
		allTools = append(allTools, parsed.Tools...)
		if parsed.NextCursor == "" || parsed.NextCursor == cursor {
			break
		}
		cursor = parsed.NextCursor
	}

	return c, allTools, nil
}

func (c *stdioMCPClient) readLoop(r io.Reader) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := readBoundedLine(br, 8*1024*1024)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {
			// 兼容 Content-Length 头帧格式。
			lenStr := strings.TrimSpace(trimmed[len("content-length:"):])
			n, pErr := strconv.Atoi(lenStr)
			if pErr == nil && n > 0 && n <= 4*1024*1024 {
				for {
					hdr, hErr := readBoundedLine(br, 64*1024)
					if hErr != nil || strings.TrimSpace(hdr) == "" {
						break
					}
				}
				payload := make([]byte, n)
				if _, rErr := io.ReadFull(br, payload); rErr == nil {
					c.dispatchFrame(payload)
				}
			}
		} else if strings.HasPrefix(trimmed, "{") {
			c.dispatchFrame([]byte(trimmed))
		}
		if err != nil {
			return
		}
	}
}

func readBoundedLine(br *bufio.Reader, maxBytes int) (string, error) {
	var buf bytes.Buffer
	for {
		frag, isPrefix, err := br.ReadLine()
		if buf.Len() < maxBytes {
			rem := maxBytes - buf.Len()
			if len(frag) > rem {
				buf.Write(frag[:rem])
			} else {
				buf.Write(frag)
			}
		}
		if !isPrefix || err != nil {
			return buf.String(), err
		}
	}
}

func (c *stdioMCPClient) dispatchFrame(raw []byte) {
	var resp jsonRPCResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return
	}
	if len(resp.ID) == 0 {
		return
	}
	var idNum uint64
	if err := json.Unmarshal(resp.ID, &idNum); err != nil {
		var idStr string
		if sErr := json.Unmarshal(resp.ID, &idStr); sErr == nil {
			idNum, _ = strconv.ParseUint(idStr, 10, 64)
		}
	}
	if idNum == 0 {
		return
	}
	c.pendMu.Lock()
	ch, ok := c.pending[idNum]
	if ok {
		delete(c.pending, idNum)
	}
	c.pendMu.Unlock()
	if ok {
		ch <- resp
	}
}

func (c *stdioMCPClient) failAllPending() {
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	c.closed = true
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

func (c *stdioMCPClient) rpcRequest(ctx context.Context, method string, params any, expectReply bool) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var id uint64
	var replyCh chan jsonRPCResponse
	if expectReply {
		id = c.seq.Add(1)
		replyCh = make(chan jsonRPCResponse, 1)
		c.pendMu.Lock()
		if c.closed {
			c.pendMu.Unlock()
			return nil, errors.New("mcp stdio client closed")
		}
		c.pending[id] = replyCh
		c.pendMu.Unlock()
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if expectReply {
		payload["id"] = id
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')

	c.writeMu.Lock()
	_, wErr := c.stdin.Write(encoded)
	c.writeMu.Unlock()
	if wErr != nil {
		if expectReply {
			c.pendMu.Lock()
			delete(c.pending, id)
			c.pendMu.Unlock()
		}
		return nil, wErr
	}
	if !expectReply {
		return nil, nil
	}

	select {
	case resp, ok := <-replyCh:
		if !ok {
			return nil, errors.New("mcp stdio subprocess exited before replying")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp stdio error %d: %s", resp.Error.Code, config.Redact(resp.Error.Message, c.secrets))
		}
		return resp.Result, nil
	case <-reqCtx.Done():
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, reqCtx.Err()
	}
}

func (c *stdioMCPClient) Call(ctx context.Context, toolName string, arguments json.RawMessage) (ToolResult, error) {
	argsObj := json.RawMessage(`{}`)
	if len(bytes.TrimSpace(arguments)) > 0 {
		argsObj = arguments
	}
	rawRes, err := c.rpcRequest(ctx, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": argsObj,
	}, true)
	if err != nil {
		return ToolResult{}, err
	}
	return decodeMCPCallPayload(rawRes), nil
}

func (c *stdioMCPClient) Close() error {
	if c == nil {
		return nil
	}
	c.writeMu.Lock()
	_ = c.stdin.Close()
	c.writeMu.Unlock()

	select {
	case <-c.doneCh:
		return nil
	case <-time.After(150 * time.Millisecond):
	}
	if c.cmd != nil && c.cmd.Process != nil && c.cmd.Process.Pid > 0 {
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
	}
	<-c.doneCh
	return nil
}

func decodeMCPCallPayload(raw json.RawMessage) ToolResult {
	var parsed mcpCallResult
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ToolResult{Output: string(raw)}
	}
	var parts []string
	for _, blk := range parsed.Content {
		if blk.Text != "" {
			parts = append(parts, blk.Text)
		}
	}
	out := strings.Join(parts, "\n")
	if out == "" && len(parsed.Content) == 0 {
		out = string(raw)
	}
	return ToolResult{
		Output:  out,
		IsError: parsed.IsError,
	}
}

func formatMCPToolName(serverID, toolName string) string {
	cleanSrv := sanitizeToolIdentifier(serverID)
	cleanTool := sanitizeToolIdentifier(toolName)
	full := "mcp__" + cleanSrv + "__" + cleanTool
	if len(full) <= 64 {
		return full
	}
	hashSuffix := sha256HexBytes([]byte(full))[:8]
	return full[:55] + "_" + hashSuffix
}

func sanitizeToolIdentifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	res := strings.Trim(b.String(), "_")
	if res == "" {
		return "tool"
	}
	return res
}
