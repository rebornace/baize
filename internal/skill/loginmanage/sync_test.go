package loginmanage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/skill/loginmanage"
	"github.com/rebornace/baize/internal/store"
)

func TestSyncWritesManagedPackage(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_sendSms", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_logout", Enabled: true},
		{ConnectorID: "auth", Name: "listThings", Enabled: true},
	})

	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}

	skillPath := filepath.Join(managed, "login-auth", "SKILL.md")
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, want := range []string{
		"managed: true",
		"managed_kind: connector_login",
		"managed_connector_id: auth",
		"name: login-auth",
		"AuthController_phoneLogin",
		"AuthController_sendSms",
		"由连接器自动维护",
		"不要再调用",
		"立刻使用下方列出的登录相关工具",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("SKILL.md missing %q\n%s", want, content)
		}
	}
	if strings.Contains(content, "baize") || strings.Contains(strings.ToLower(content), "baize") {
		t.Fatal("vendor-neutral copy must not mention product name")
	}
	if _, err := os.Stat(filepath.Join(managed, "login-auth", "workflow.yaml")); !os.IsNotExist(err) {
		t.Fatalf("workflow.yaml must not exist, err=%v", err)
	}
}

func TestSyncSkipsNonManagedConflict(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	pkg := filepath.Join(userSkills, "login-auth")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "---\nname: login-auth\ndescription: user fork\ntools: [custom]\n---\n\n# User skill\n"
	if err := os.WriteFile(filepath.Join(pkg, "SKILL.md"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
	})

	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(pkg, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("user package must stay unchanged\ngot:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(managed, "login-auth")); !os.IsNotExist(err) {
		t.Fatal("must not write managed package when user conflict exists")
	}
}

func TestSyncConflictDeletesResidualManaged(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}
	managedPkg := filepath.Join(managed, "login-auth")
	if _, err := os.Stat(managedPkg); err != nil {
		t.Fatal(err)
	}

	userPkg := filepath.Join(userSkills, "login-auth")
	if err := os.MkdirAll(userPkg, 0o755); err != nil {
		t.Fatal(err)
	}
	userMD := "---\nname: login-auth\ndescription: user fork\ntools: [custom]\n---\n\n# User\n"
	if err := os.WriteFile(filepath.Join(userPkg, "SKILL.md"), []byte(userMD), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managedPkg); !os.IsNotExist(err) {
		t.Fatal("residual managed package must be deleted on user conflict")
	}
	got, err := os.ReadFile(filepath.Join(userPkg, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != userMD {
		t.Fatal("user package must remain")
	}
}

func TestSyncDeletesWhenTypeBecomesMCP(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(managed, "login-auth")
	if _, err := os.Stat(pkg); err != nil {
		t.Fatal(err)
	}

	st.UpsertConnector(store.Connector{ID: "auth", Type: "mcp"})
	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Fatal("managed package must be deleted when connector type becomes mcp")
	}
}

func TestSyncDeletesWhenNoTools(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "http"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(managed, "login-auth")
	if _, err := os.Stat(pkg); err != nil {
		t.Fatal(err)
	}

	st.ReplaceConnectorTools("auth", nil)
	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Fatal("managed package must be deleted when no login tools remain")
	}
}

func TestSyncAllSkipsMCP(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	st.UpsertConnector(store.Connector{ID: "mcp1", Type: "mcp"})
	st.ReplaceConnectorTools("mcp1", []store.Tool{
		{ConnectorID: "mcp1", Name: "oauthLogin", Enabled: true},
	})

	if err := loginmanage.SyncAll(st, managed, userSkills); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(managed, "login-auth", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(managed, "login-mcp1")); !os.IsNotExist(err) {
		t.Fatal("mcp connectors must not get managed login skills")
	}
}

func TestSyncAllRemovesOrphanManagedLogin(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	userSkills := filepath.Join(root, "user")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, managed, userSkills, "auth"); err != nil {
		t.Fatal(err)
	}

	orphan := filepath.Join(managed, "login-gone")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	orphanMD := "---\nname: login-gone\ndescription: orphan\ntools: [x]\nmanaged: true\nmanaged_kind: connector_login\nmanaged_connector_id: gone\n---\n\n# orphan\n"
	if err := os.WriteFile(filepath.Join(orphan, "SKILL.md"), []byte(orphanMD), 0o644); err != nil {
		t.Fatal(err)
	}

	st.UpsertConnector(store.Connector{ID: "auth", Type: "mcp"})
	if err := loginmanage.SyncAll(st, managed, userSkills); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(managed, "login-auth")); !os.IsNotExist(err) {
		t.Fatal("type-changed connector package must be removed by SyncAll")
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan managed connector_login must be removed by SyncAll")
	}
}
