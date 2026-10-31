package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"frpc-mesh/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS instances(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  server_addr TEXT NOT NULL,
  server_port INTEGER NOT NULL DEFAULT 7000,
  auth_method TEXT NOT NULL DEFAULT 'token',
  auth_token TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 0,
  transport TEXT NOT NULL DEFAULT '{}',
  admin TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS proxies(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'proxy',
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  local_ip TEXT NOT NULL DEFAULT '127.0.0.1',
  local_port INTEGER NOT NULL DEFAULT 0,
  remote_port INTEGER NOT NULL DEFAULT 0,
  custom_domains TEXT NOT NULL DEFAULT '',
  subdomain TEXT NOT NULL DEFAULT '',
  locations TEXT NOT NULL DEFAULT '',
  http_user TEXT NOT NULL DEFAULT '',
  http_password TEXT NOT NULL DEFAULT '',
  host_header_rewrite TEXT NOT NULL DEFAULT '',
  use_encryption INTEGER NOT NULL DEFAULT 0,
  use_compression INTEGER NOT NULL DEFAULT 0,
  secret_key TEXT NOT NULL DEFAULT '',
  server_name TEXT NOT NULL DEFAULT '',
  bind_addr TEXT NOT NULL DEFAULT '127.0.0.1',
  bind_port INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings(k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS users(username TEXT PRIMARY KEY, password_hash TEXT NOT NULL);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// 旧版本未开外键级联，删除服务器可能残留孤儿代理行，此处一次性清理
	if _, err := s.db.Exec(`DELETE FROM proxies WHERE instance_id NOT IN (SELECT id FROM instances)`); err != nil {
		return err
	}
	return s.migrateV2()
}

// migrateV2 为旧库补充 bandwidth_limit 列
func (s *Store) migrateV2() error {
	rows, err := s.db.Query(`PRAGMA table_info(proxies)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	has := false
	hasEnabled := false
	hasRemark := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "bandwidth_limit" {
			has = true
		}
		if name == "enabled" {
			hasEnabled = true
		}
		if name == "remark" {
			hasRemark = true
		}
	}
	if !has {
		if _, err := s.db.Exec(`ALTER TABLE proxies ADD COLUMN bandwidth_limit TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !hasEnabled {
		if _, err := s.db.Exec(`ALTER TABLE proxies ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if !hasRemark {
		if _, err := s.db.Exec(`ALTER TABLE proxies ADD COLUMN remark TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	rows2, err := s.db.Query(`PRAGMA table_info(instances)`)
	if err != nil {
		return err
	}
	defer rows2.Close()
	hasMp := false
	hasInstRemark := false
	hasDomain := false
	for rows2.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows2.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "manage_port" {
			hasMp = true
		}
		if name == "remark" {
			hasInstRemark = true
		}
		if name == "domain" {
			hasDomain = true
		}
	}
	if !hasMp {
		if _, err := s.db.Exec(`ALTER TABLE instances ADD COLUMN manage_port INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !hasInstRemark {
		if _, err := s.db.Exec(`ALTER TABLE instances ADD COLUMN remark TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !hasDomain {
		if _, err := s.db.Exec(`ALTER TABLE instances ADD COLUMN domain TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------- settings ----------

func (s *Store) GetSetting(k, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT v FROM settings WHERE k=?`, k).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO settings(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

func (s *Store) GetSettings() model.Settings {
	st := model.Settings{
		AutoStart:       s.GetSetting("auto_start", "true") == "true",
		AutoUpdateCheck: s.GetSetting("auto_update_check", "true") == "true",
	}
	return st
}

func (s *Store) SaveSettings(st model.Settings) error {
	v := "false"
	if st.AutoStart {
		v = "true"
	}
	if err := s.SetSetting("auto_start", v); err != nil {
		return err
	}
	v = "false"
	if st.AutoUpdateCheck {
		v = "true"
	}
	return s.SetSetting("auto_update_check", v)
}

// ---------- users ----------

func (s *Store) GetUserHash(username string) (string, error) {
	var h string
	err := s.db.QueryRow(`SELECT password_hash FROM users WHERE username=?`, username).Scan(&h)
	return h, err
}

func (s *Store) UpsertUser(username, hash string) error {
	_, err := s.db.Exec(`INSERT INTO users(username,password_hash) VALUES(?,?) ON CONFLICT(username) DO UPDATE SET password_hash=excluded.password_hash`, username, hash)
	return err
}

func (s *Store) CountUsers() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n
}

// ---------- instances ----------

func scanInstance(row interface{ Scan(...any) error }) (*model.Instance, error) {
	var in model.Instance
	var enabled int
	var transport, admin string
	err := row.Scan(&in.ID, &in.Name, &in.ServerAddr, &in.ServerPort, &in.AuthMethod,
		&in.AuthToken, &enabled, &transport, &admin, &in.CreatedAt, &in.UpdatedAt, &in.ManagePort, &in.Remark, &in.Domain)
	if err != nil {
		return nil, err
	}
	in.Enabled = enabled == 1
	json.Unmarshal([]byte(transport), &in.Transport)
	json.Unmarshal([]byte(admin), &in.AdminCfg)
	return &in, nil
}

const instanceCols = `id,name,server_addr,server_port,auth_method,auth_token,enabled,transport,admin,created_at,updated_at,manage_port,remark,domain`

func (s *Store) ListInstances() ([]*model.Instance, error) {
	rows, err := s.db.Query(`SELECT ` + instanceCols + ` FROM instances ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Instance
	for rows.Next() {
		in, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

func (s *Store) GetInstance(id int64) (*model.Instance, error) {
	row := s.db.QueryRow(`SELECT `+instanceCols+` FROM instances WHERE id=?`, id)
	return scanInstance(row)
}

func (s *Store) CreateInstance(in *model.Instance) error {
	tb, _ := json.Marshal(in.Transport)
	ab, _ := json.Marshal(in.AdminCfg)
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	res, err := s.db.Exec(`INSERT INTO instances(name,server_addr,server_port,auth_method,auth_token,enabled,transport,admin,created_at,updated_at,manage_port,remark,domain) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.Name, in.ServerAddr, in.ServerPort, in.AuthMethod, in.AuthToken, enabled, string(tb), string(ab), now(), now(), in.ManagePort, in.Remark, in.Domain)
	if err != nil {
		return err
	}
	in.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateInstance(in *model.Instance) error {
	tb, _ := json.Marshal(in.Transport)
	ab, _ := json.Marshal(in.AdminCfg)
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`UPDATE instances SET name=?,server_addr=?,server_port=?,auth_method=?,auth_token=?,enabled=?,transport=?,admin=?,updated_at=?,manage_port=?,remark=?,domain=? WHERE id=?`,
		in.Name, in.ServerAddr, in.ServerPort, in.AuthMethod, in.AuthToken, enabled, string(tb), string(ab), now(), in.ManagePort, in.Remark, in.Domain, in.ID)
	return err
}

func (s *Store) SetInstanceEnabled(id int64, enabled bool) error {
	e := 0
	if enabled {
		e = 1
	}
	_, err := s.db.Exec(`UPDATE instances SET enabled=?, updated_at=? WHERE id=?`, e, now(), id)
	return err
}

func (s *Store) DeleteInstance(id int64) error {
	_, err := s.db.Exec(`DELETE FROM instances WHERE id=?`, id)
	return err
}

func (s *Store) ListEnabledInstances() ([]*model.Instance, error) {
	rows, err := s.db.Query(`SELECT ` + instanceCols + ` FROM instances WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Instance
	for rows.Next() {
		in, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

// ---------- proxies ----------

func scanProxy(row interface{ Scan(...any) error }) (*model.Proxy, error) {
	var p model.Proxy
	var enc, comp int
	err := row.Scan(&p.ID, &p.InstanceID, &p.Kind, &p.Name, &p.Type, &p.LocalIP, &p.LocalPort,
		&p.RemotePort, &p.CustomDomains, &p.Subdomain, &p.Locations, &p.HTTPUser, &p.HTTPPassword,
		&p.HostHeaderRewrite, &enc, &comp, &p.BandwidthLimit, &p.SecretKey, &p.ServerName, &p.BindAddr, &p.BindPort,
		&p.CreatedAt, &p.UpdatedAt, &p.Enabled, &p.Remark)
	if err != nil {
		return nil, err
	}
	p.UseEncryption = enc == 1
	p.UseCompression = comp == 1
	return &p, nil
}

const proxyCols = `id,instance_id,kind,name,type,local_ip,local_port,remote_port,custom_domains,subdomain,locations,http_user,http_password,host_header_rewrite,use_encryption,use_compression,bandwidth_limit,secret_key,server_name,bind_addr,bind_port,created_at,updated_at,enabled,remark`

func (s *Store) ListProxies(instanceID int64) ([]*model.Proxy, error) {
	rows, err := s.db.Query(`SELECT `+proxyCols+` FROM proxies WHERE instance_id=? ORDER BY id`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Proxy
	for rows.Next() {
		p, err := scanProxy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetProxy(id int64) (*model.Proxy, error) {
	row := s.db.QueryRow(`SELECT `+proxyCols+` FROM proxies WHERE id=?`, id)
	return scanProxy(row)
}

func (s *Store) CreateProxy(p *model.Proxy) error {
	enc, comp := 0, 0
	if p.UseEncryption {
		enc = 1
	}
	if p.UseCompression {
		comp = 1
	}
	res, err := s.db.Exec(`INSERT INTO proxies(instance_id,kind,name,type,local_ip,local_port,remote_port,custom_domains,subdomain,locations,http_user,http_password,host_header_rewrite,use_encryption,use_compression,bandwidth_limit,secret_key,server_name,bind_addr,bind_port,created_at,updated_at,enabled,remark) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.InstanceID, p.Kind, p.Name, p.Type, p.LocalIP, p.LocalPort, p.RemotePort, p.CustomDomains,
		p.Subdomain, p.Locations, p.HTTPUser, p.HTTPPassword, p.HostHeaderRewrite, enc, comp,
		p.BandwidthLimit, p.SecretKey, p.ServerName, p.BindAddr, p.BindPort, now(), now(), boolInt(p.Enabled), p.Remark)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateProxy(p *model.Proxy) error {
	enc, comp := 0, 0
	if p.UseEncryption {
		enc = 1
	}
	if p.UseCompression {
		comp = 1
	}
	_, err := s.db.Exec(`UPDATE proxies SET kind=?,name=?,type=?,local_ip=?,local_port=?,remote_port=?,custom_domains=?,subdomain=?,locations=?,http_user=?,http_password=?,host_header_rewrite=?,use_encryption=?,use_compression=?,bandwidth_limit=?,secret_key=?,server_name=?,bind_addr=?,bind_port=?,updated_at=?,enabled=?,remark=? WHERE id=?`,
		p.Kind, p.Name, p.Type, p.LocalIP, p.LocalPort, p.RemotePort, p.CustomDomains, p.Subdomain,
		p.Locations, p.HTTPUser, p.HTTPPassword, p.HostHeaderRewrite, enc, comp,
		p.BandwidthLimit, p.SecretKey, p.ServerName, p.BindAddr, p.BindPort, now(), boolInt(p.Enabled), p.Remark, p.ID)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) SetProxyEnabled(id int64, enabled bool) error {
	_, err := s.db.Exec(`UPDATE proxies SET enabled=?, updated_at=? WHERE id=?`, boolInt(enabled), now(), id)
	return err
}

func (s *Store) DeleteProxy(id int64) error {
	_, err := s.db.Exec(`DELETE FROM proxies WHERE id=?`, id)
	return err
}


// CountActiveProxies 仅统计启用中的代理（仪表盘口径）
func (s *Store) CountActiveProxies(kind string) int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE kind=? AND enabled=1`, kind).Scan(&n)
	return n
}

func (s *Store) CountProxiesByInstance(instanceID int64) int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE instance_id=?`, instanceID).Scan(&n)
	return n
}

func (s *Store) CountInstances() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM instances`).Scan(&n)
	return n
}

// UsedInstanceAdminPorts 返回除 excludeID 外所有实例占用的 admin 端口
func (s *Store) UsedAdminPorts(excludeID int64) []int {
	rows, err := s.db.Query(`SELECT id,admin FROM instances`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ports []int
	for rows.Next() {
		var id int64
		var admin string
		if rows.Scan(&id, &admin) == nil && id != excludeID {
			var a model.Admin
			if json.Unmarshal([]byte(admin), &a) == nil && a.Enabled {
				ports = append(ports, a.Port)
			}
		}
	}
	return ports
}

var ErrNotFound = sql.ErrNoRows
