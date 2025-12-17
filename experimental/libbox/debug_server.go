package libbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DebugServer 调试 HTTP 服务器（集成到 Libbox）
type DebugServer struct {
	server    *http.Server
	listener  net.Listener
	basePath  string
	port      int
	running   bool
	startTime time.Time
	mu        sync.RWMutex
}

var (
	debugServerInstance *DebugServer
	debugServerMu       sync.Mutex
)

// newDebugServer 创建调试服务器实例（内部使用，不导出到 iOS）
func newDebugServer() *DebugServer {
	debugServerMu.Lock()
	defer debugServerMu.Unlock()

	if debugServerInstance == nil {
		debugServerInstance = &DebugServer{
			port: 8618,
		}
	}
	return debugServerInstance
}

// SetBasePath 设置 App Group 容器路径
func (s *DebugServer) SetBasePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.basePath = path
}

// Start 启动调试服务器
func (s *DebugServer) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	s.port = port

	mux := http.NewServeMux()
	s.setupRoutes(mux)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 🔥 明确绑定到 0.0.0.0，允许局域网访问
	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)
	var err error
	s.listener, err = net.Listen("tcp4", listenAddr)
	if err != nil {
		// 如果 tcp4 失败，尝试 tcp
		s.listener, err = net.Listen("tcp", listenAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
		}
	}

	s.running = true
	s.startTime = time.Now()

	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}
	}()

	return nil
}

// Stop 停止调试服务器
func (s *DebugServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if s.server != nil {
		s.server.Close()
	}
	if s.listener != nil {
		s.listener.Close()
	}

	s.running = false
	s.server = nil
	s.listener = nil
	return nil
}

// Restart 重启服务器
func (s *DebugServer) Restart(newPort int) error {
	s.Stop()
	time.Sleep(100 * time.Millisecond)
	return s.Start(newPort)
}

// IsRunning 检查是否运行中
func (s *DebugServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetPort 获取当前端口
func (s *DebugServer) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

// GetStatusJSON 获取状态 JSON
func (s *DebugServer) GetStatusJSON() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"running":   s.running,
		"port":      s.port,
		"localIP":   getLocalIP(),
		"startTime": "",
	}
	if s.running {
		status["startTime"] = s.startTime.Format(time.RFC3339)
	}

	data, _ := json.Marshal(status)
	return string(data)
}

func (s *DebugServer) setupRoutes(mux *http.ServeMux) {
	// 首页
	mux.HandleFunc("/", s.handleIndex)
	// 日志
	mux.HandleFunc("/logs/tunnel", s.handleTunnelLog)
	mux.HandleFunc("/logs/stderr", s.handleStderrLog)
	// 配置
	mux.HandleFunc("/config/generated", s.handleGeneratedConfig)
	mux.HandleFunc("/config/userdefaults", s.handleUserDefaults)
	mux.HandleFunc("/configs", s.handleListConfigs)
	// 状态
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	// 控制
	mux.HandleFunc("/api/control/restart", s.handleRestart)
	mux.HandleFunc("/api/control/stop", s.handleStop)
}

func (s *DebugServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <title>SingForge VPN Debug</title>
    <style>
        body { font-family: -apple-system, sans-serif; padding: 20px; background: #1e1e1e; color: #d4d4d4; max-width: 1200px; margin: 0 auto; }
        h1 { color: #4fc3f7; }
        h2 { color: #81c784; margin-top: 30px; }
        a { color: #4fc3f7; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .info { background: #2d2d2d; padding: 10px 15px; border-radius: 5px; margin: 10px 0; border-left: 3px solid #ffa726; }
        .endpoint { margin: 10px 0; padding: 15px; background: #2d2d2d; border-radius: 5px; border-left: 3px solid #81c784; }
        .endpoint strong { color: #fff; }
        .success { color: #4caf50; }
        pre { background: #2d2d2d; padding: 15px; border-radius: 5px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>🔧 SingForge VPN 调试服务器 (Go)</h1>
    <div class="info">
        <strong>端口:</strong> %d | <strong>状态:</strong> <span class="success">运行中 (VPN Extension - Go)</span>
    </div>
    
    <h2>📋 日志接口</h2>
    <div class="endpoint"><a href="/logs/tunnel"><strong>GET /logs/tunnel</strong></a><br><small>隧道日志</small></div>
    <div class="endpoint"><a href="/logs/stderr"><strong>GET /logs/stderr</strong></a><br><small>Stderr 日志</small></div>
    
    <h2>⚙️ 配置接口</h2>
    <div class="endpoint"><a href="/config/generated"><strong>GET /config/generated</strong></a><br><small>生成的配置</small></div>
    <div class="endpoint"><a href="/configs"><strong>GET /configs</strong></a><br><small>配置文件列表</small></div>
    
    <h2>📊 状态接口</h2>
    <div class="endpoint"><a href="/status"><strong>GET /status</strong></a><br><small>服务器状态</small></div>
    
    <h2>🎮 控制接口</h2>
    <div class="endpoint"><strong>POST /api/control/restart</strong><br><small>重启服务器</small></div>
    <div class="endpoint"><strong>POST /api/control/stop</strong><br><small>停止服务器</small></div>
    
    <h2>💡 特性</h2>
    <ul>
        <li>✅ 集成到 Libbox，无符号冲突</li>
        <li>✅ 在 VPN Extension 中后台运行</li>
        <li>✅ VPN 开启时持续可用</li>
    </ul>
</body>
</html>`, s.port)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (s *DebugServer) handleTunnelLog(w http.ResponseWriter, r *http.Request) {
	logFile := filepath.Join(s.basePath, "tunnel.log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		http.Error(w, "日志文件不存在: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

func (s *DebugServer) handleStderrLog(w http.ResponseWriter, r *http.Request) {
	// 尝试多个路径
	paths := []string{
		filepath.Join(s.basePath, "cache", "stderr.log"),
		filepath.Join(s.basePath, "Library", "Caches", "stderr.log"),
	}

	for _, path := range paths {
		if content, err := os.ReadFile(path); err == nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(content)
			return
		}
	}

	http.Error(w, "Stderr 日志文件不存在", http.StatusNotFound)
}

func (s *DebugServer) handleGeneratedConfig(w http.ResponseWriter, r *http.Request) {
	configFile := filepath.Join(s.basePath, "singbox", "config.json")
	content, err := os.ReadFile(configFile)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error": "配置文件不存在"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(content)
}

func (s *DebugServer) handleUserDefaults(w http.ResponseWriter, r *http.Request) {
	// UserDefaults 内容需要从 plist 读取
	plistPath := filepath.Join(s.basePath, "Library", "Preferences", "group.com.singforge.vpn.plist")
	content, err := os.ReadFile(plistPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error": "UserDefaults 文件不存在"}`))
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write(content)
}

func (s *DebugServer) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.basePath, "singbox")

	var configs []map[string]interface{}

	err := filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
			configs = append(configs, map[string]interface{}{
				"name":     info.Name(),
				"size":     info.Size(),
				"modified": info.ModTime().Format(time.RFC3339),
			})
		}
		return nil
	})

	if err != nil {
		configs = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(configs)
}

func (s *DebugServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(s.GetStatusJSON()))
}

func (s *DebugServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	s.handleStatus(w, r)
}

func (s *DebugServer) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Port = s.port
	}
	if req.Port <= 0 {
		req.Port = s.port
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Restart(req.Port)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true, "message": "restarting"}`))
}

func (s *DebugServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Stop()
	}()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true, "message": "stopping"}`))
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// ========== iOS 绑定入口 ==========

// DebugServerStart 启动调试服务器（iOS 调用）
func DebugServerStart(port int) error {
	server := newDebugServer()
	return server.Start(port)
}

// DebugServerStop 停止调试服务器（iOS 调用）
func DebugServerStop() error {
	server := newDebugServer()
	return server.Stop()
}

// DebugServerRestart 重启调试服务器（iOS 调用）
func DebugServerRestart(port int) error {
	server := newDebugServer()
	return server.Restart(port)
}

// DebugServerSetBasePath 设置基础路径（iOS 调用）
func DebugServerSetBasePath(path string) {
	server := newDebugServer()
	server.SetBasePath(path)
}

// DebugServerIsRunning 检查是否运行中（iOS 调用）
func DebugServerIsRunning() bool {
	server := newDebugServer()
	return server.IsRunning()
}

// DebugServerGetPort 获取端口（iOS 调用）
func DebugServerGetPort() int {
	server := newDebugServer()
	return server.GetPort()
}

// DebugServerGetStatusJSON 获取状态 JSON（iOS 调用）
func DebugServerGetStatusJSON() string {
	server := newDebugServer()
	return server.GetStatusJSON()
}

// UploadConfig 上传配置文件
func (s *DebugServer) UploadConfig(name string, content []byte) error {
	configDir := filepath.Join(s.basePath, "singbox")
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, name)
	return os.WriteFile(configPath, content, 0644)
}

// DownloadConfig 下载配置文件
func (s *DebugServer) DownloadConfig(name string) ([]byte, error) {
	configPath := filepath.Join(s.basePath, "singbox", name)
	return os.ReadFile(configPath)
}

// DeleteConfig 删除配置文件
func (s *DebugServer) DeleteConfig(name string) error {
	configPath := filepath.Join(s.basePath, "singbox", name)
	return os.Remove(configPath)
}

// 添加文件上传处理
func (s *DebugServer) handleUploadConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析 multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.UploadConfig(handler.Filename, content); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}
