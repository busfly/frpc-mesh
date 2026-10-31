package model

type Transport struct {
	Protocol             string `json:"protocol"`
	TCPMux               bool   `json:"tcp_mux"`
	PoolCount            int    `json:"pool_count"`
	Heartbeat            int    `json:"heartbeat"`
	HeartbeatTimeout     int    `json:"heartbeat_timeout"`
	TcpMuxKeepalive      int    `json:"tcp_mux_keepalive"`
	DialTimeout          int    `json:"dial_timeout"`
	DialServerKeepalive  int    `json:"dial_server_keepalive"`
	TLSEnable            bool   `json:"tls_enable"`
	ProxyProtocolVersion string `json:"proxy_protocol_version"`
	CustomStunServer     string `json:"custom_stun_server"`
}

type Admin struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type Runtime struct {
	Status        string `json:"status"`
	PID           int    `json:"pid"`
	StartedAt     string `json:"started_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	LastError     string `json:"last_error"`
	// FrpcVersion 该实例当前进程实际使用的 frpc 版本
	FrpcVersion string `json:"frpc_version"`
}

type Instance struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ServerAddr string `json:"server_addr"`
	ServerPort int    `json:"server_port"`
	AuthMethod string `json:"auth_method"`
	AuthToken  string `json:"auth_token"`
	ManagePort int    `json:"manage_port"`
	Remark     string `json:"remark"`
	Domain     string `json:"domain"`
	// FrpcVersion 为空表示跟随全局默认版本
	FrpcVersion string    `json:"frpc_version"`
	Enabled     bool      `json:"enabled"`
	Transport   Transport `json:"transport"`
	AdminCfg    Admin     `json:"admin"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	Runtime     *Runtime  `json:"runtime,omitempty"`
}

type Proxy struct {
	ID                int64  `json:"id"`
	InstanceID        int64  `json:"instance_id"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	Enabled           bool   `json:"enabled"`
	LocalIP           string `json:"local_ip"`
	LocalPort         int    `json:"local_port"`
	RemotePort        int    `json:"remote_port"`
	CustomDomains     string `json:"custom_domains"`
	Subdomain         string `json:"subdomain"`
	Locations         string `json:"locations"`
	HTTPUser          string `json:"http_user"`
	HTTPPassword      string `json:"http_password"`
	HostHeaderRewrite string `json:"host_header_rewrite"`
	UseEncryption     bool   `json:"use_encryption"`
	UseCompression    bool   `json:"use_compression"`
	BandwidthLimit    string `json:"bandwidth_limit"`
	SecretKey         string `json:"secret_key"`
	ServerName        string `json:"server_name"`
	BindAddr          string `json:"bind_addr"`
	BindPort          int    `json:"bind_port"`
	Remark            string `json:"remark"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type Settings struct {
	AutoStart       bool   `json:"auto_start"`
	AutoUpdateCheck bool   `json:"auto_update_check"`
	Version         string `json:"version,omitempty"`
}

// BackupInstance 备份文件中的单个服务器及其代理
type BackupInstance struct {
	Instance
	Proxies []*Proxy `json:"proxies"`
}

// Backup 备份/恢复文件的整体结构
type Backup struct {
	App        string           `json:"app"`
	Version    int              `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Instances  []BackupInstance `json:"instances"`
}
