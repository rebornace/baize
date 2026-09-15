package loginmanage_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/skill/loginmanage"
	"github.com/rebornace/baize/internal/store"
)

func testMemoryBlobs(t *testing.T) blob.Store {
	t.Helper()
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSyncWritesManagedPackage(t *testing.T) {
	blobs := testMemoryBlobs(t)
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_sendSms", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_logout", Enabled: true},
		{ConnectorID: "auth", Name: "listThings", Enabled: true},
	})

	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}

	key := blob.SkillObjectKey("managed", "login-auth", "SKILL.md")
	raw, err := blobs.Get(context.Background(), key)
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
	if _, err := blobs.Get(context.Background(), blob.SkillObjectKey("managed", "login-auth", "workflow.yaml")); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("workflow.yaml must not exist, err=%v", err)
	}
}

func TestSyncManagedVisibleInCatalogAndDeletesWithConnector(t *testing.T) {
	blobs := testMemoryBlobs(t)
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})

	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog(nil, "", blobs)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cat.Get("login-auth")
	if !ok || p.Source != skill.SourceManaged {
		t.Fatalf("Catalog missing login-auth after Sync: %+v ok=%v", p, ok)
	}

	if err := st.DeleteConnector("auth"); err != nil {
		t.Fatal(err)
	}
	if err := loginmanage.SyncAll(st, blobs); err != nil {
		t.Fatal(err)
	}
	key := blob.SkillObjectKey("managed", "login-auth", "SKILL.md")
	if _, err := blobs.Get(context.Background(), key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("managed blob should be gone after connector delete, err=%v", err)
	}
	if err := cat.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("login-auth"); ok {
		t.Fatal("Catalog should drop login-auth after managed blob deleted")
	}
}

func TestSyncSkipsNonManagedConflict(t *testing.T) {
	blobs := testMemoryBlobs(t)
	ctx := context.Background()
	original := "---\nname: login-auth\ndescription: user fork\ntools: [custom]\n---\n\n# User skill\n"
	if err := blobs.Put(ctx, blob.SkillObjectKey("user", "login-auth", "SKILL.md"), []byte(original), "text/markdown"); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
	})

	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}

	got, err := blobs.Get(ctx, blob.SkillObjectKey("user", "login-auth", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("user package must stay unchanged\ngot:\n%s", got)
	}
	if _, err := blobs.Get(ctx, blob.SkillObjectKey("managed", "login-auth", "SKILL.md")); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("must not write managed package when user conflict exists")
	}
}

func TestSyncConflictDeletesResidualManaged(t *testing.T) {
	blobs := testMemoryBlobs(t)
	ctx := context.Background()
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	managedKey := blob.SkillObjectKey("managed", "login-auth", "SKILL.md")
	if _, err := blobs.Get(ctx, managedKey); err != nil {
		t.Fatal(err)
	}

	userMD := "---\nname: login-auth\ndescription: user fork\ntools: [custom]\n---\n\n# User\n"
	if err := blobs.Put(ctx, blob.SkillObjectKey("user", "login-auth", "SKILL.md"), []byte(userMD), "text/markdown"); err != nil {
		t.Fatal(err)
	}

	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, managedKey); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("residual managed package must be deleted on user conflict")
	}
	got, err := blobs.Get(ctx, blob.SkillObjectKey("user", "login-auth", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != userMD {
		t.Fatal("user package must remain")
	}
}

func TestSyncDeletesWhenTypeBecomesMCP(t *testing.T) {
	blobs := testMemoryBlobs(t)
	ctx := context.Background()
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	key := blob.SkillObjectKey("managed", "login-auth", "SKILL.md")
	if _, err := blobs.Get(ctx, key); err != nil {
		t.Fatal(err)
	}

	st.UpsertConnector(store.Connector{ID: "auth", Type: "mcp"})
	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("managed package must be deleted when connector type becomes mcp")
	}
}

func TestSyncDeletesWhenNoTools(t *testing.T) {
	blobs := testMemoryBlobs(t)
	ctx := context.Background()
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "http"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	key := blob.SkillObjectKey("managed", "login-auth", "SKILL.md")
	if _, err := blobs.Get(ctx, key); err != nil {
		t.Fatal(err)
	}

	st.ReplaceConnectorTools("auth", nil)
	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("managed package must be deleted when no login tools remain")
	}
}

func TestSyncAllSkipsMCP(t *testing.T) {
	blobs := testMemoryBlobs(t)
	ctx := context.Background()
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	st.UpsertConnector(store.Connector{ID: "mcp1", Type: "mcp"})
	st.ReplaceConnectorTools("mcp1", []store.Tool{
		{ConnectorID: "mcp1", Name: "oauthLogin", Enabled: true},
	})

	if err := loginmanage.SyncAll(st, blobs); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, blob.SkillObjectKey("managed", "login-auth", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, blob.SkillObjectKey("managed", "login-mcp1", "SKILL.md")); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("mcp connectors must not get managed login skills")
	}
}

func TestSyncAllRemovesOrphanManagedLogin(t *testing.T) {
	blobs := testMemoryBlobs(t)
	ctx := context.Background()
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "phoneLogin", Enabled: true},
	})
	if err := loginmanage.SyncConnector(st, blobs, "auth"); err != nil {
		t.Fatal(err)
	}

	orphanMD := "---\nname: login-gone\ndescription: orphan\ntools: [x]\nmanaged: true\nmanaged_kind: connector_login\nmanaged_connector_id: gone\n---\n\n# orphan\n"
	if err := blobs.Put(ctx, blob.SkillObjectKey("managed", "login-gone", "SKILL.md"), []byte(orphanMD), "text/markdown"); err != nil {
		t.Fatal(err)
	}

	st.UpsertConnector(store.Connector{ID: "auth", Type: "mcp"})
	if err := loginmanage.SyncAll(st, blobs); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, blob.SkillObjectKey("managed", "login-auth", "SKILL.md")); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("type-changed connector package must be removed by SyncAll")
	}
	if _, err := blobs.Get(ctx, blob.SkillObjectKey("managed", "login-gone", "SKILL.md")); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal("orphan managed connector_login must be removed by SyncAll")
	}
}
