package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"frpc-mesh/internal/model"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "frpc-mesh.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// 旧库（无 frpc_version 列）打开后应自动补列，已有数据读回为空串（跟随默认）
func TestInstanceFrpcVersionMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE instances(
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, server_addr TEXT NOT NULL,
		server_port INTEGER NOT NULL DEFAULT 7000, auth_method TEXT NOT NULL DEFAULT 'token',
		auth_token TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
		transport TEXT NOT NULL DEFAULT '{}', admin TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO instances(name,server_addr,server_port,created_at,updated_at) VALUES('老服务器','1.2.3.4',7000,'x','x')`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("旧库迁移失败: %v", err)
	}
	defer st.Close()
	list, err := st.ListInstances()
	if err != nil {
		t.Fatalf("迁移后读取失败: %v", err)
	}
	if len(list) != 1 || list[0].FrpcVersion != "" {
		t.Fatalf("迁移后应读回空版本，实际 %+v", list)
	}
	// 幂等：再开一次不报错
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	st2.Close()
}

func TestInstanceFrpcVersionRoundTrip(t *testing.T) {
	st := openTemp(t)
	in := &model.Instance{Name: "s1", ServerAddr: "1.1.1.1", ServerPort: 7000, FrpcVersion: "0.70.0"}
	if err := st.CreateInstance(in); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetInstance(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrpcVersion != "0.70.0" {
		t.Fatalf("创建后读回 %q", got.FrpcVersion)
	}
	got.FrpcVersion = ""
	if err := st.UpdateInstance(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := st.GetInstance(in.ID)
	if got2.FrpcVersion != "" {
		t.Fatalf("更新回默认后读回 %q", got2.FrpcVersion)
	}
}
