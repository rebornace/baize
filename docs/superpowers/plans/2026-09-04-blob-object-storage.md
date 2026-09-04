# 对象存储多源（BlobStore）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 把分析报告产物的 HTML 字节存储从本地磁盘抽象为可替换的 blob 驱动（默认 `file` 零依赖，新增 `s3` 兼容 AWS S3/MinIO/OSS/COS），读取走代理读，URL/ACL 不变。

**架构：** 新增通用 `internal/blob` 包（`Store` 接口 `Put/Get/Delete` + 驱动注册表，复刻 X3 `middleware` 范式），三驱动 `file`/`memory`/`s3`。`artifact.Store` 业务接口加 `ctx`，字节走 blob、元数据仍以 SQL 为权威；写失败回滚字节。bootstrap 按 `storage.driver` 配置装配。

**技术栈：** Go 1.25、`github.com/minio/minio-go/v7`（Apache-2.0）、既有 `store.SQLBackend`/`dbutil`、`httptest` 伪造 S3 端点。

**设计规格：** `docs/superpowers/specs/2026-09-04-blob-object-storage-design.md`

**环境约束（每个 shell 命令前设置）：**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
```

- commit message 用 `git commit -F <file>`，文件用 `[System.IO.File]::WriteAllText($p,$msg,[System.Text.UTF8Encoding]::new($false))` 写入（无 BOM），避免 PowerShell 中文乱码。
- 每任务结束 `go build ./...` 与相关包 `go test -count=1` 必须绿。

---

## 文件结构

**创建：**

| 文件 | 职责 |
|------|------|
| `internal/blob/blob.go` | `Store` 接口、`ErrNotFound`、`Options`/`FileOptions`/`S3Options` |
| `internal/blob/registry.go` | 驱动注册表：`DriverFactory`、`RegisterDriver`、`Open`、`ListDrivers` |
| `internal/blob/registry_test.go` | 未知 driver 报错 |
| `internal/blob/drivers_test.go` | 经注册表打开 file/memory 往返（blank import 两驱动） |
| `internal/blob/file/file.go` | 本地磁盘驱动（`file`，默认），`init()` 自注册 |
| `internal/blob/file/file_test.go` | file 驱动单测 |
| `internal/blob/memory/memory.go` | 内存驱动（`memory`，测试用），`init()` 自注册 |
| `internal/blob/memory/memory_test.go` | memory 驱动单测 |
| `internal/blob/s3/s3.go` | S3 兼容驱动（minio-go），`init()` 自注册 |
| `internal/blob/s3/s3_test.go` | httptest 伪造 S3 端点单测 |
| `internal/artifact/blob_store.go` | artifact 业务实现：字节走 blob、元数据走 SQL |
| `internal/config/storage_test.go` | storage 配置段测试 |

**修改：**

| 文件 | 改动 |
|------|------|
| `internal/artifact/store.go` | 接口 `PutHTML`/`Get` 加 `ctx context.Context` |
| `internal/artifact/file_store.go` | **删除**（字节逻辑迁入 `blob_store.go` + `blob/file`） |
| `internal/artifact/store_test.go` | 改用 `blob/file` + `artifact.NewStore`，调用带 ctx；加回滚测试 |
| `internal/analysis/tool.go` | `art.PutHTML(ctx, runID, html)` |
| `internal/analysis/tool_test.go` | helper 改用 `artifact.NewStore` |
| `internal/api/server.go` | `handleGetArtifact` 用 `s.Artifacts.Get(r.Context(), id)` |
| `internal/api/server_artifacts_test.go` | helper 改用 `NewStore`；`PutHTML` 带 ctx |
| `internal/bootstrap/bootstrap.go` | T4 先切到 file blob 驱动（`blob.Open`+`artifact.NewStore`，blank import `blob/file`，删 `artifactDataDir`）；T5 改为 `openBlobStore` 按 `storage` 配置选驱动 |
| `cmd/baize/main.go` | blank import `internal/blob/s3` |
| `internal/config/config.go` | 新增 `Storage` 配置段 + 默认归一化 + `StorageUseSSL()` |
| `configs/demo.yaml`、`configs/minimal.yaml` | 加 `storage` 段注释示例 |
| `tests/integration/analysis_page_test.go` | helper 改用 `NewStore` |
| `go.mod` / `go.sum` | 新增 minio-go 依赖 |

---
---

## 任务 1：config `storage` 配置段

**文件：**
- 修改：`internal/config/config.go`（`Config` 结构体加字段；`applyDefaults` 加归一化；加 `StorageUseSSL()`）
- 测试：`internal/config/storage_test.go`（创建）

- [ ] **步骤 1：编写失败的测试**

创建 `internal/config/storage_test.go`：

```go
package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadStorageDefaults(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Driver != "file" {
		t.Fatalf("driver=%q want file", cfg.Storage.Driver)
	}
	s := cfg.Storage.S3
	if s.Prefix != "baize" || !cfg.StorageUseSSL() {
		t.Fatalf("s3 defaults wrong: prefix=%q useSSL=%v", s.Prefix, cfg.StorageUseSSL())
	}
	if s.AccessKeyEnv != "S3_ACCESS_KEY" || s.SecretKeyEnv != "S3_SECRET_KEY" {
		t.Fatalf("key env defaults wrong: %q/%q", s.AccessKeyEnv, s.SecretKeyEnv)
	}
	if s.AutoCreateBucket {
		t.Fatalf("auto_create_bucket default must be false")
	}
	if s.PathStyle {
		t.Fatalf("path_style default must be false")
	}
}

func TestLoadStorageS3Explicit(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nstorage:\n  driver: s3\n  file:\n    root_dir: /var/lib/baize\n  s3:\n    endpoint: minio.local:9000\n    region: us-east-1\n    bucket: baize-prod\n    prefix: prod\n    access_key_env: MINIO_AK\n    secret_key_env: MINIO_SK\n    use_ssl: false\n    path_style: true\n    auto_create_bucket: true\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	st := cfg.Storage
	if st.Driver != "s3" || st.File.RootDir != "/var/lib/baize" {
		t.Fatalf("storage driver/root wrong: %+v", st)
	}
	s := st.S3
	if s.Endpoint != "minio.local:9000" || s.Region != "us-east-1" || s.Bucket != "baize-prod" ||
		s.Prefix != "prod" || s.AccessKeyEnv != "MINIO_AK" || s.SecretKeyEnv != "MINIO_SK" ||
		s.UseSSL || !s.PathStyle || !s.AutoCreateBucket {
		t.Fatalf("s3 config not parsed: %+v", s)
	}
	if cfg.StorageUseSSL() {
		t.Fatalf("use_ssl=false should be respected")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/config/ -run TestLoadStorage -count=1`
预期：编译失败，`cfg.Storage undefined`。

- [ ] **步骤 3：在 `Config` 结构体加 `Storage` 字段**

在 `internal/config/config.go` 的 `Middleware struct{...}` 字段之后（`MockTicket` 之前）插入：

```go
	Storage struct {
		Driver string `yaml:"driver"` // file（默认）| s3
		File   struct {
			RootDir string `yaml:"root_dir"` // 空则用 dataDir
		} `yaml:"file"`
		S3 struct {
			Endpoint         string `yaml:"endpoint"`
			Region           string `yaml:"region"`
			Bucket           string `yaml:"bucket"`
			Prefix           string `yaml:"prefix"`
			AccessKeyEnv     string `yaml:"access_key_env"`
			SecretKeyEnv     string `yaml:"secret_key_env"`
			UseSSL           *bool  `yaml:"use_ssl"`
			PathStyle        bool   `yaml:"path_style"`
			AutoCreateBucket bool   `yaml:"auto_create_bucket"`
		} `yaml:"s3"`
	} `yaml:"storage"`
```

> `UseSSL` 用 `*bool` 以区分「未设置（默认 true）」与「显式 false」。

- [ ] **步骤 4：在 `applyDefaults` 加归一化**

在 `applyDefaults` 内（middleware 默认值块之后、`cfg.Skills.UserDir` 之前）加入：

```go
	if strings.TrimSpace(cfg.Storage.Driver) == "" {
		cfg.Storage.Driver = "file"
	}
	if cfg.Storage.S3.Prefix == "" {
		cfg.Storage.S3.Prefix = "baize"
	}
	if cfg.Storage.S3.AccessKeyEnv == "" {
		cfg.Storage.S3.AccessKeyEnv = "S3_ACCESS_KEY"
	}
	if cfg.Storage.S3.SecretKeyEnv == "" {
		cfg.Storage.S3.SecretKeyEnv = "S3_SECRET_KEY"
	}
	if cfg.Storage.S3.UseSSL == nil {
		v := true
		cfg.Storage.S3.UseSSL = &v
	}
```

- [ ] **步骤 5：加 `StorageUseSSL()` 访问器**

在 `CompactEnabled()` 方法附近加入：

```go
// StorageUseSSL reports whether the S3 driver should use TLS (default true).
func (c Config) StorageUseSSL() bool {
	if c.Storage.S3.UseSSL == nil {
		return true
	}
	return *c.Storage.S3.UseSSL
}
```

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/config/ -count=1`
预期：PASS（含既有 config 测试）。

- [ ] **步骤 7：Commit**

```powershell
git add internal/config/config.go internal/config/storage_test.go
git commit -m "feat(config): 新增 storage 驱动配置段（file/s3）与默认归一化"
```

---
## 任务 2：blob 核心（接口 / Options / 注册表）

**文件：**
- 创建：`internal/blob/blob.go`
- 创建：`internal/blob/registry.go`
- 测试：`internal/blob/registry_test.go`（创建）

- [ ] **步骤 1：编写失败的测试**

创建 `internal/blob/registry_test.go`：

```go
package blob_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
)

func TestOpenUnknownDriver(t *testing.T) {
	_, err := blob.Open(context.Background(), "nope", blob.Options{})
	if err == nil || !strings.Contains(err.Error(), `unknown blob driver`) {
		t.Fatalf("want unknown driver error, got %v", err)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("duplicate registration should panic")
		}
	}()
	blob.RegisterDriver("dup-test", func(_ context.Context, _ blob.Options) (blob.Store, error) {
		return nil, nil
	})
	blob.RegisterDriver("dup-test", func(_ context.Context, _ blob.Options) (blob.Store, error) {
		return nil, nil
	})
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/blob/ -count=1`
预期：编译失败，`github.com/rebornace/baize/internal/blob` 包不存在。

- [ ] **步骤 3：创建 `internal/blob/blob.go`**

```go
// Package blob defines a swappable object-storage abstraction for binary
// content (report artifacts, and later connector specs / skills / attachments).
// Drivers register themselves via RegisterDriver, mirroring the middleware
// driver pattern; "file" is the zero-dependency default.
package blob

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Get/Delete when an object does not exist. Drivers
// map their underlying "not found" error to this sentinel (wrap with fmt.Errorf
// "...: %w", blob.ErrNotFound).
var ErrNotFound = errors.New("blob: object not found")

// Store is a key/value binary object store. Keys are driver-relative and use "/"
// separators (e.g. "artifacts/art_xxx.html"). There is no directory notion;
// drivers create parent paths as needed.
type Store interface {
	// Put writes data under key. contentType sets the S3 Content-Type header;
	// the file driver may ignore it.
	Put(ctx context.Context, key string, data []byte, contentType string) error
	// Get returns the object bytes. A missing object returns an error wrapping
	// ErrNotFound.
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete removes the object. Deleting a missing object is a no-op success.
	Delete(ctx context.Context, key string) error
}

// Options configures a blob driver. Only the section for the selected driver is
// read.
type Options struct {
	File FileOptions
	S3   S3Options
}

// FileOptions configures the local-disk driver.
type FileOptions struct {
	// RootDir is the directory under which keys are materialized. Required for
	// the file driver.
	RootDir string
}

// S3Options configures the S3-compatible driver (AWS S3 / MinIO / OSS / COS).
type S3Options struct {
	Endpoint   string // e.g. s3.amazonaws.com, play.min.io, oss-cn-hangzhou.aliyuncs.com
	Region     string
	Bucket     string
	Prefix     string // prepended to every key for multi-tenant/env isolation
	AccessKey  string // resolved from the environment by bootstrap
	SecretKey  string
	UseSSL     bool
	PathStyle  bool // true for MinIO/self-hosted; false for AWS
	AutoCreate bool // MakeBucket on startup if missing
}
```

- [ ] **步骤 4：创建 `internal/blob/registry.go`**

```go
package blob

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// DriverFactory constructs a Store from options.
type DriverFactory func(ctx context.Context, opts Options) (Store, error)

var (
	driversMu sync.RWMutex
	drivers   = map[string]DriverFactory{}
)

// RegisterDriver registers a blob driver by name (called from init()).
// Registering the same name twice panics (programmer error).
func RegisterDriver(name string, f DriverFactory) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if _, ok := drivers[name]; ok {
		panic(fmt.Sprintf("blob: driver %q already registered", name))
	}
	drivers[name] = f
}

func lookupDriver(name string) (DriverFactory, error) {
	driversMu.RLock()
	defer driversMu.RUnlock()
	f, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown blob driver %q (registered: %v)", name, ListDrivers())
	}
	return f, nil
}

// ListDrivers returns registered driver names, sorted.
func ListDrivers() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	names := make([]string, 0, len(drivers))
	for n := range drivers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Open builds a Store for the named driver.
func Open(ctx context.Context, driver string, opts Options) (Store, error) {
	f, err := lookupDriver(driver)
	if err != nil {
		return nil, err
	}
	return f(ctx, opts)
}
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/blob/ -count=1`
预期：PASS（`TestOpenUnknownDriver`、`TestRegisterDuplicatePanics`）。

- [ ] **步骤 6：Commit**

```powershell
git add internal/blob/blob.go internal/blob/registry.go internal/blob/registry_test.go
git commit -m "feat(blob): 通用对象存储接口与驱动注册表"
```

---
## 任务 3：file 与 memory 驱动

**文件：**
- 创建：`internal/blob/file/file.go`
- 创建：`internal/blob/file/file_test.go`
- 创建：`internal/blob/memory/memory.go`
- 创建：`internal/blob/memory/memory_test.go`
- 创建：`internal/blob/drivers_test.go`（经注册表打开两驱动）

- [ ] **步骤 1：编写 file 驱动测试**

创建 `internal/blob/file/file_test.go`：

```go
package file_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
)

func TestFilePutGetRoundTrip(t *testing.T) {
	root := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "artifacts/art_abc.html", []byte("<html>ok</html>"), "text/html; charset=utf-8"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "artifacts/art_abc.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<html>ok</html>" {
		t.Fatalf("got %q", got)
	}
	// 文件确实落在 <root>/artifacts/ 下（路径与今日布局一致）。
	if _, err := os.Stat(filepath.Join(root, "artifacts", "art_abc.html")); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}
}

func TestFileGetMissing(t *testing.T) {
	root := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(context.Background(), "artifacts/nope.html")
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestFileDeleteIdempotent(t *testing.T) {
	root := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "a/b/c.bin", []byte("x"), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "a/b/c.bin"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "a/b/c.bin"); err != nil { // 再删不报错
		t.Fatalf("delete missing should be nil, got %v", err)
	}
	if _, err := s.Get(ctx, "a/b/c.bin"); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/blob/file/ -count=1`
预期：编译失败（`file` 包不存在）。

- [ ] **步骤 3：创建 `internal/blob/file/file.go`**

```go
// Package file is the zero-dependency local-disk blob driver. It materializes
// keys as files under a root directory, mirroring the on-disk layout used before
// blob abstraction (so upgrades need no migration).
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rebornace/baize/internal/blob"
)

func init() {
	blob.RegisterDriver("file", func(_ context.Context, opts blob.Options) (blob.Store, error) {
		if opts.File.RootDir == "" {
			return nil, fmt.Errorf("blob file driver: file.root_dir is required")
		}
		return &store{root: opts.File.RootDir}, nil
	})
}

type store struct {
	root string
}

var _ blob.Store = (*store)(nil)

func (s *store) path(key string) string {
	return filepath.Join(s.root, filepath.FromSlash(key))
}

func (s *store) Put(_ context.Context, key string, data []byte, _ string) error {
	p := s.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	return nil
}

func (s *store) Get(_ context.Context, key string) ([]byte, error) {
	b, err := os.ReadFile(s.path(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
		}
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	return b, nil
}

func (s *store) Delete(_ context.Context, key string) error {
	err := os.Remove(s.path(key))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", key, err)
	}
	return nil
}
```

- [ ] **步骤 4：创建 memory 驱动测试**

创建 `internal/blob/memory/memory_test.go`：

```go
package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

func TestMemoryPutGetDelete(t *testing.T) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "k1", []byte("v1"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "k1")
	if err != nil || string(got) != "v1" {
		t.Fatalf("get=%q err=%v", got, err)
	}
	if _, err := s.Get(ctx, "missing"); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "k1"); err != nil { // 幂等
		t.Fatalf("delete missing should be nil, got %v", err)
	}
}
```

- [ ] **步骤 5：创建 `internal/blob/memory/memory.go`**

```go
// Package memory is an in-process blob driver, primarily for tests and future
// in-process deployments.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/rebornace/baize/internal/blob"
)

func init() {
	blob.RegisterDriver("memory", func(_ context.Context, _ blob.Options) (blob.Store, error) {
		return &store{m: map[string][]byte{}}, nil
	})
}

type store struct {
	mu sync.RWMutex
	m  map[string][]byte
}

var _ blob.Store = (*store)(nil)

func (s *store) Put(_ context.Context, key string, data []byte, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.m[key] = cp
	return nil
}

func (s *store) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.m[key]
	if !ok {
		return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func (s *store) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}
```

- [ ] **步骤 6：创建注册表驱动列举测试**

创建 `internal/blob/drivers_test.go`（`package blob_test`，blank import 两驱动）：

```go
package blob_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

func TestListDriversIncludesFileAndMemory(t *testing.T) {
	names := blob.ListDrivers()
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if !got["file"] || !got["memory"] {
		t.Fatalf("want file+memory registered, got %v", names)
	}
}

func TestOpenFileViaRegistry(t *testing.T) {
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), "x", []byte("y"), ""); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **步骤 7：运行测试验证通过**

运行：`go test ./internal/blob/... -count=1`
预期：PASS（registry + file + memory + drivers 全绿）。

- [ ] **步骤 8：Commit**

```powershell
git add internal/blob/file internal/blob/memory internal/blob/drivers_test.go
git commit -m "feat(blob): file 本地磁盘驱动与 memory 内存驱动"
```

---
## 任务 4：artifact 适配（接口加 ctx、字节走 blob）

**文件：**
- 修改：`internal/artifact/store.go`（接口加 `ctx`）
- 创建：`internal/artifact/blob_store.go`
- 删除：`internal/artifact/file_store.go`
- 修改：`internal/artifact/store_test.go`（改用 blob 驱动 + 回滚测试）
- 修改：`internal/analysis/tool.go:98`、`internal/analysis/tool_test.go`
- 修改：`internal/api/server.go:2026`、`internal/api/server_artifacts_test.go`
- 修改：`tests/integration/analysis_page_test.go`
- 修改：`internal/bootstrap/bootstrap.go`（本任务先把装配切到 file blob 驱动，保证全仓可编译；配置驱动选择在任务 5）

- [ ] **步骤 1：改写 artifact 测试（先红）**

把 `internal/artifact/store_test.go` 整体替换为：

```go
package artifact_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/store"
)

func newTestStore(t *testing.T, root string) artifact.Store {
	t.Helper()
	dbPath := filepath.Join(root, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	as, err := artifact.NewStore(blobs, st)
	if err != nil {
		t.Fatal(err)
	}
	return as
}

func TestStorePutGetRoundTrip(t *testing.T) {
	as := newTestStore(t, t.TempDir())
	id, err := as.PutHTML(context.Background(), "run_1", "<html><body>ok</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	html, runID, err := as.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run_1" || !strings.Contains(html, "ok") {
		t.Fatalf("got run=%s html=%s", runID, html)
	}
}

func TestGetNotFound(t *testing.T) {
	as := newTestStore(t, t.TempDir())
	if _, _, err := as.Get(context.Background(), "art_missing"); err == nil {
		t.Fatalf("want error for missing artifact")
	}
}

// recordingStore wraps a blob.Store and records Delete keys to verify rollback.
type recordingStore struct {
	blob.Store
	deleted []string
}

func (r *recordingStore) Delete(ctx context.Context, key string) error {
	r.deleted = append(r.deleted, key)
	return r.Store.Delete(ctx, key)
}

func TestPutHTMLRollsBackBlobOnMetadataFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	// 用 memory blob（包一层记录 Delete），构造完 artifact store 后关闭 DB，
	// 迫使 INSERT 元数据失败 → 应回滚已写入的字节。
	mem, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recordingStore{Store: mem}
	as, err := artifact.NewStore(rec, st)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close() // 此后任何 SQL Exec 都失败

	if _, err := as.PutHTML(context.Background(), "run_1", "<html>x</html>"); err == nil {
		t.Fatalf("want metadata insert failure")
	}
	if len(rec.deleted) != 1 || !strings.HasPrefix(rec.deleted[0], "artifacts/art_") {
		t.Fatalf("want exactly one rollback delete under artifacts/, got %v", rec.deleted)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/artifact/ -count=1`
预期：编译失败（`artifact.NewStore` 未定义、`PutHTML` 参数数量不符）。

- [ ] **步骤 3：改 `internal/artifact/store.go` 接口加 ctx**

整体替换为：

```go
package artifact

import "context"

// Store persists analysis-page HTML artifacts. Bytes live in a blob.Store;
// metadata (id -> run_id) lives in the SQL backend.
type Store interface {
	PutHTML(ctx context.Context, runID string, html string) (id string, err error)
	Get(ctx context.Context, id string) (html string, runID string, err error)
}
```

- [ ] **步骤 4：创建 `internal/artifact/blob_store.go`**

```go
package artifact

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/dbutil"
	"github.com/rebornace/baize/internal/store"
)

const (
	artifactKeyPrefix   = "artifacts/"
	artifactContentType = "text/html; charset=utf-8"
)

const sqliteArtifactsSchema = `
CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
`

// BlobStore stores HTML bytes in a blob.Store and metadata in SQL.
type BlobStore struct {
	blobs    blob.Store
	db       *sql.DB
	postgres bool
}

var _ Store = (*BlobStore)(nil)

// NewStore creates an artifact Store backed by blobs for bytes and the SQL
// backend for metadata.
func NewStore(blobs blob.Store, backend store.SQLBackend) (*BlobStore, error) {
	if blobs == nil {
		return nil, fmt.Errorf("blob store is nil")
	}
	if backend == nil {
		return nil, fmt.Errorf("sql backend is nil")
	}
	db := backend.DB()
	if db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	postgres := backend.Dialect() == store.DialectPostgres
	schema := sqliteArtifactsSchema
	if postgres {
		schema = dbutil.RebindPostgres(schema)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate artifacts schema: %w", err)
	}
	return &BlobStore{blobs: blobs, db: db, postgres: postgres}, nil
}

func (s *BlobStore) q(query string) string {
	if s.postgres {
		return dbutil.RebindPostgres(query)
	}
	return query
}

func (s *BlobStore) key(id string) string { return artifactKeyPrefix + id + ".html" }

// PutHTML writes html to the blob store and records its association with runID
// in SQL. On metadata failure the just-written bytes are rolled back.
func (s *BlobStore) PutHTML(ctx context.Context, runID string, html string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := "art_" + hex.EncodeToString(b)
	key := s.key(id)

	if err := s.blobs.Put(ctx, key, []byte(html), artifactContentType); err != nil {
		return "", fmt.Errorf("put artifact blob: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		s.q(`INSERT INTO artifacts (id, run_id, created_at) VALUES (?, ?, ?)`),
		id, runID, time.Now().Unix()); err != nil {
		if delErr := s.blobs.Delete(ctx, key); delErr != nil {
			log.Printf("artifact: rollback delete %s after metadata failure: %v", key, delErr)
		}
		return "", fmt.Errorf("record artifact metadata: %w", err)
	}
	return id, nil
}

// Get returns the HTML content and runID for an artifact id.
func (s *BlobStore) Get(ctx context.Context, id string) (string, string, error) {
	var runID string
	err := s.db.QueryRowContext(ctx, s.q(`SELECT run_id FROM artifacts WHERE id = ?`), id).Scan(&runID)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("artifact not found")
	}
	if err != nil {
		return "", "", err
	}
	data, err := s.blobs.Get(ctx, s.key(id))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return "", "", fmt.Errorf("artifact not found")
		}
		return "", "", fmt.Errorf("get artifact blob: %w", err)
	}
	return string(data), runID, nil
}
```

- [ ] **步骤 5：删除 `internal/artifact/file_store.go`**

```powershell
Remove-Item internal/artifact/file_store.go
```

- [ ] **步骤 6：更新调用点 `internal/analysis/tool.go`**

把 `id, err := art.PutHTML(runID, html)` 改为：

```go
		id, err := art.PutHTML(ctx, runID, html)
```

- [ ] **步骤 7：更新调用点 `internal/api/server.go` 的 `handleGetArtifact`**

把 `html, runID, err := s.Artifacts.Get(id)` 改为：

```go
	html, runID, err := s.Artifacts.Get(r.Context(), id)
```

- [ ] **步骤 8：更新三处测试 helper**

`internal/analysis/tool_test.go`：把 `testFileStore` 改为（并把 import 块加上 `context`、`internal/blob` 与 blank import `internal/blob/file`）：

```go
func testFileStore(t *testing.T) artifact.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	as, err := artifact.NewStore(blobs, st)
	if err != nil {
		t.Fatal(err)
	}
	return as
}
```

`internal/api/server_artifacts_test.go`：`testArtifactStore` 同样改为 `blob.Open(..., "file", RootDir: dir)` + `artifact.NewStore(blobs, st)`（import 加 `context`、`internal/blob`、blank `internal/blob/file`）；并把 `artID, err := srv.Artifacts.PutHTML(runID, html)` 改为：

```go
	artID, err := srv.Artifacts.PutHTML(context.Background(), runID, html)
```

`tests/integration/analysis_page_test.go`：把构造 `artifact.NewFileStore(...)` 处改为先 `blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})` 再 `artifact.NewStore(blobs, st)`（import 加 `context`、`internal/blob`、blank `internal/blob/file`）。

- [ ] **步骤 9：更新 bootstrap（本任务先切到 file blob，保证全仓编译）**

> 任务 4 删除了 `NewFileStore`，bootstrap 是生产调用点，必须在本任务同步改到可编译；配置驱动选择在任务 5 引入。

在 `internal/bootstrap/bootstrap.go`：

1. import 块加 `"github.com/rebornace/baize/internal/blob"` 与 blank import `_ "github.com/rebornace/baize/internal/blob/file"`。
2. 把 artifact 装配块改为（根目录取 `dataDir(cfg)`，兜底 `./data`）：

```go
	if sqlBackend != nil {
		root := dataDir(cfg)
		if root == "" {
			root = "./data"
		}
		blobStore, err := blob.Open(context.Background(), "file", blob.Options{
			File: blob.FileOptions{RootDir: root},
		})
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open blob store: %w", err)
		}
		artStore, err := artifact.NewStore(blobStore, sqlBackend)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open artifact store: %w", err)
		}
		reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))
		srv.Artifacts = artStore
	}
```

3. 删除现已无人调用的 `artifactDataDir` 函数：

```go
func artifactDataDir(cfg config.Config) string {
	if dir := dataDir(cfg); dir != "" {
		return filepath.Join(dir, "artifacts")
	}
	return "./data/artifacts"
}
```

若 `filepath` 因此在 bootstrap 中不再被使用，移除该 import（`dataDir` 本身仍用 `filepath.Dir`，通常仍需要——以 `go build` 结果为准）。

- [ ] **步骤 10：运行测试验证通过**

运行：`go build ./... && go test ./internal/artifact/ ./internal/analysis/ ./internal/api/ ./internal/bootstrap/ ./tests/integration/ -count=1`
预期：PASS。

- [ ] **步骤 11：Commit**

```powershell
git add internal/artifact internal/analysis internal/api tests/integration
git commit -m "feat(artifact): 产物字节改走 blob.Store，接口加 ctx，元数据仍以 SQL 为准"
```

---
## 任务 5：bootstrap 装配 + configs 示例

**文件：**
- 修改：`internal/bootstrap/bootstrap.go`（任务 4 已切到 file blob；本任务改为按 `storage` 配置选驱动，新增 `openBlobStore`/`s3CredFromEnv`）
- 修改：`configs/demo.yaml`、`configs/minimal.yaml`

- [ ] **步骤 1：确认 import（任务 4 已加 blank import）**

任务 4 已在 `internal/bootstrap/bootstrap.go` 引入 `"github.com/rebornace/baize/internal/blob"` 与 blank import `_ "github.com/rebornace/baize/internal/blob/file"`，本任务无需再加。

- [ ] **步骤 2：把任务 4 的临时 file 装配替换为配置驱动**

把任务 4 写入的 artifact 装配块（`if sqlBackend != nil { root := dataDir(cfg); ... blob.Open(..., "file", ...) ... }`）替换为：

```go
	if sqlBackend != nil {
		blobStore, err := openBlobStore(context.Background(), cfg)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open blob store: %w", err)
		}
		artStore, err := artifact.NewStore(blobStore, sqlBackend)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open artifact store: %w", err)
		}
		reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))
		srv.Artifacts = artStore
	}
```

- [ ] **步骤 3：新增 `openBlobStore` 与 `s3CredsFromEnv`**

在 `redisPasswordFromEnv` 函数附近加入：

```go
// openBlobStore builds the configured object-storage driver. The file driver
// roots under dataDir when storage.file.root_dir is unset, preserving the
// historical <dataDir>/artifacts layout.
func openBlobStore(ctx context.Context, cfg config.Config) (blob.Store, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Storage.Driver))
	if driver == "" {
		driver = "file"
	}
	opts := blob.Options{
		File: blob.FileOptions{RootDir: cfg.Storage.File.RootDir},
		S3: blob.S3Options{
			Endpoint:   cfg.Storage.S3.Endpoint,
			Region:     cfg.Storage.S3.Region,
			Bucket:     cfg.Storage.S3.Bucket,
			Prefix:     cfg.Storage.S3.Prefix,
			AccessKey:  s3CredFromEnv(cfg.Storage.S3.AccessKeyEnv),
			SecretKey:  s3CredFromEnv(cfg.Storage.S3.SecretKeyEnv),
			UseSSL:     cfg.StorageUseSSL(),
			PathStyle:  cfg.Storage.S3.PathStyle,
			AutoCreate: cfg.Storage.S3.AutoCreateBucket,
		},
	}
	if driver == "file" && opts.File.RootDir == "" {
		opts.File.RootDir = dataDir(cfg)
		if opts.File.RootDir == "" {
			opts.File.RootDir = "./data"
		}
	}
	return blob.Open(ctx, driver, opts)
}

// s3CredFromEnv resolves an S3 credential from the named environment variable.
func s3CredFromEnv(env string) string {
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}
```

- [ ] **步骤 4：configs 加注释示例**

在 `configs/demo.yaml` 与 `configs/minimal.yaml` 的 `middleware:` 段附近追加（注释态，默认 file 不影响现有部署）：

```yaml
# 对象存储：分析报告产物 HTML 字节存放位置。
# driver: file（默认，本地磁盘 <dataDir>/artifacts）| s3（AWS S3/MinIO/OSS/COS 兼容）
# storage:
#   driver: file
#   file:
#     root_dir: ""          # 留空则用 dataDir
#   s3:
#     endpoint: "minio.local:9000"
#     region: ""
#     bucket: "baize"
#     prefix: "baize"
#     access_key_env: S3_ACCESS_KEY   # 密钥仅从环境变量读取
#     secret_key_env: S3_SECRET_KEY
#     use_ssl: false
#     path_style: true                # MinIO/自建设 true；AWS 设 false
#     auto_create_bucket: false
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/bootstrap/ ./internal/config/ -count=1`
预期：PASS。

- [ ] **步骤 6：Commit**

```powershell
git add internal/bootstrap/bootstrap.go configs/demo.yaml configs/minimal.yaml
git commit -m "feat(bootstrap): 按 storage 配置装配 blob 驱动并接入产物存储"
```

---
## 任务 6：s3 驱动（minio-go）+ 自注册

**文件：**
- 创建：`internal/blob/s3/s3.go`
- 创建：`internal/blob/s3/s3_test.go`
- 修改：`cmd/baize/main.go`（blank import）

- [ ] **步骤 1：拉取依赖**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go get github.com/minio/minio-go/v7
```

预期：`go.mod` 新增 `github.com/minio/minio-go/v7`（Apache-2.0）。

- [ ] **步骤 2：编写 s3 驱动测试（httptest 伪造 S3 端点）**

创建 `internal/blob/s3/s3_test.go`：

```go
package s3_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/s3"
)

// fakeS3 is a minimal path-style S3 endpoint sufficient for minio-go
// PutObject/GetObject/RemoveObject/BucketExists/MakeBucket.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte // object name (no bucket) -> bytes
	putCT   map[string]string
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}, putCT: map[string]string{}} }

func (f *fakeS3) handler(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	bucket := parts[0]
	object := ""
	if len(parts) > 1 {
		object = parts[1]
	}
	switch r.Method {
	case http.MethodHead: // BucketExists
		if bucket == "baize" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	case http.MethodPut:
		if object == "" { // MakeBucket
			w.WriteHeader(http.StatusOK)
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.objects[object] = b
		f.putCT[object] = r.Header.Get("Content-Type")
		f.mu.Unlock()
		w.Header().Set("ETag", `"etag"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		f.mu.Lock()
		b, ok := f.objects[object]
		f.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `<?xml version="1.0"?><Error><Code>NoSuchKey</Code><Message>missing</Message><Key>%s</Key></Error>`, object)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	case http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, object)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func openAgainst(t *testing.T, srv *httptest.Server, bucket string, autoCreate bool) (blob.Store, error) {
	t.Helper()
	endpoint := strings.TrimPrefix(srv.URL, "http://")
	return blob.Open(context.Background(), "s3", blob.Options{S3: blob.S3Options{
		Endpoint:   endpoint,
		Bucket:     bucket,
		Prefix:     "baize",
		AccessKey:  "ak",
		SecretKey:  "sk",
		UseSSL:     false,
		PathStyle:  true,
		AutoCreate: autoCreate,
	}})
}

func TestS3PutGetRoundTrip(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()

	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "artifacts/art_1.html", []byte("<html>ok</html>"), "text/html; charset=utf-8"); err != nil {
		t.Fatal(err)
	}
	// 对象名含 prefix，Content-Type 透传。
	fake.mu.Lock()
	ct := fake.putCT["baize/artifacts/art_1.html"]
	_, present := fake.objects["baize/artifacts/art_1.html"]
	fake.mu.Unlock()
	if !present {
		t.Fatalf("object not stored under prefix; keys=%v", mapKeys(fake))
	}
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type=%q", ct)
	}
	got, err := s.Get(ctx, "artifacts/art_1.html")
	if err != nil || string(got) != "<html>ok</html>" {
		t.Fatalf("get=%q err=%v", got, err)
	}
}

func TestS3GetMissingMapsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(context.Background(), "artifacts/nope.html")
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestS3DeleteIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "k", []byte("v"), ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "k"); err != nil { // 幂等
		t.Fatalf("delete missing should be nil, got %v", err)
	}
}

func TestS3MissingBucketNoAutoCreateFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	if _, err := openAgainst(t, srv, "other-bucket", false); err == nil {
		t.Fatalf("want error when bucket missing and auto_create=false")
	}
}

func TestS3MissingBucketAutoCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	// fake 对未知 bucket HEAD 404，但 PUT /bucket 仍 200（MakeBucket）。
	s, err := openAgainst(t, srv, "fresh-bucket", true)
	if err != nil {
		t.Fatalf("auto-create should succeed, got %v", err)
	}
	if err := s.Put(context.Background(), "k", []byte("v"), ""); err != nil {
		t.Fatal(err)
	}
}

func TestS3RequiresCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	endpoint := strings.TrimPrefix(srv.URL, "http://")
	if _, err := blob.Open(context.Background(), "s3", blob.Options{S3: blob.S3Options{
		Endpoint: endpoint, Bucket: "baize", UseSSL: false, PathStyle: true,
	}}); err == nil {
		t.Fatalf("want error when credentials missing")
	}
}

func mapKeys(f *fakeS3) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for k := range f.objects {
		out = append(out, k)
	}
	return out
}
```

- [ ] **步骤 3：运行测试验证失败**

运行：`go test ./internal/blob/s3/ -count=1`
预期：编译失败（`internal/blob/s3` 包不存在）。

- [ ] **步骤 4：创建 `internal/blob/s3/s3.go`**

```go
// Package s3 is the S3-compatible blob driver (AWS S3 / MinIO / Aliyun OSS /
// Tencent COS) backed by github.com/minio/minio-go/v7 (Apache-2.0 client SDK).
package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rebornace/baize/internal/blob"
)

func init() {
	blob.RegisterDriver("s3", open)
}

type store struct {
	client *minio.Client
	bucket string
	prefix string // 无首尾 "/"
}

var _ blob.Store = (*store)(nil)

func open(ctx context.Context, opts blob.Options) (blob.Store, error) {
	c := opts.S3
	if c.Endpoint == "" {
		return nil, fmt.Errorf("blob s3 driver: storage.s3.endpoint is required")
	}
	if c.Bucket == "" {
		return nil, fmt.Errorf("blob s3 driver: storage.s3.bucket is required")
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return nil, fmt.Errorf("blob s3 driver: access/secret key are required (set storage.s3.access_key_env/secret_key_env)")
	}
	mopts := &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: c.UseSSL,
		Region: c.Region,
	}
	if c.PathStyle {
		mopts.BucketLookup = minio.BucketLookupPath
	}
	cl, err := minio.New(c.Endpoint, mopts)
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}

	exists, err := cl.BucketExists(ctx, c.Bucket)
	if err != nil {
		return nil, fmt.Errorf("s3 bucket %q check: %w", c.Bucket, err)
	}
	if !exists {
		if !c.AutoCreate {
			return nil, fmt.Errorf("s3 bucket %q does not exist at %q (set storage.s3.auto_create_bucket: true to create it)", c.Bucket, c.Endpoint)
		}
		if err := cl.MakeBucket(ctx, c.Bucket, minio.MakeBucketOptions{Region: c.Region}); err != nil {
			return nil, fmt.Errorf("create s3 bucket %q: %w", c.Bucket, err)
		}
	}
	return &store{client: cl, bucket: c.Bucket, prefix: strings.Trim(c.Prefix, "/")}, nil
}

func (s *store) object(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

func (s *store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, s.object(key),
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (s *store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.object(key), minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
		}
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer obj.Close()
	b, err := io.ReadAll(obj) // GetObject 的 NoSuchKey 常在 Read 时暴露
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
		}
		return nil, fmt.Errorf("s3 read %s: %w", key, err)
	}
	return b, nil
}

func (s *store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, s.object(key), minio.RemoveObjectOptions{})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	code := minio.ToErrorResponse(err).Code
	return code == "NoSuchKey" || code == "NoSuchBucket"
}
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/blob/s3/ -count=1`
预期：PASS（5 个测试）。若 minio 对 path-style 的 BucketExists/MakeBucket 路径与假端点不一致，按真实请求方法/路径调整 `fakeS3.handler`（用 `t.Logf` 打印 `r.Method`/`r.URL.Path` 定位），但**不得**改 `s3.go` 的装配语义。

- [ ] **步骤 6：main.go blank import**

在 `cmd/baize/main.go` import 块，紧跟现有 `_ "github.com/rebornace/baize/internal/middleware/redis"` 之后加：

```go
	_ "github.com/rebornace/baize/internal/blob/s3"
```

- [ ] **步骤 7：构建验证**

运行：`go build ./...`
预期：PASS。

- [ ] **步骤 8：Commit**

```powershell
git add internal/blob/s3 cmd/baize/main.go go.mod go.sum
git commit -m "feat(blob): s3 兼容驱动（minio-go，S3/MinIO/OSS/COS）与启动桶校验"
```

---
## 任务 7：全量验证、文档与收尾

**文件：**
- 修改：`docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md`（backlog 勾选）
- 视情况：`README.md`（产物存储生产集成说明，可选）

- [ ] **步骤 1：全量构建与测试**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go build ./...
go test ./... -count=1
go vet ./internal/blob/... ./internal/artifact/...
```

预期：全部 PASS，无 vet 告警。

- [ ] **步骤 2：核对 file 驱动路径兼容（无迁移）**

确认 `storage.driver` 默认 `file`、`root_dir` 留空时落到 `dataDir`，artifact key `artifacts/<id>.html` 解析为 `<dataDir>/artifacts/<id>.html`，与改造前 `artifactDataDir` 布局一致。可跑一次既有 integration 测试（`tests/integration`）佐证产物仍可经 `GET /v0/artifacts/{id}` 读到。

- [ ] **步骤 3：更新 backlog 笔记**

在 `docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md` 的「已交付」区追加一行（对象存储可替换落地，标记 X3 延期项中「对象存储」完成）：

```markdown
| **已交付** | **BlobStore 对象存储多源**（分析报告产物可换 file/s3（AWS S3/MinIO/OSS/COS）；通用 `blob.Store` + 驱动注册表，代理读；规格 `2026-09-04-blob-object-storage-design.md`） |
```

- [ ] **步骤 4：README 生产集成小节（可选但建议）**

在 README 的生产集成/部署相关章节补一小段：产物存储可通过 `storage.driver: s3` 指向 S3 兼容对象存储，密钥经 `S3_ACCESS_KEY`/`S3_SECRET_KEY` 环境变量注入；默认 file 落 `<dataDir>/artifacts`。若 README 无合适章节则跳过，不强行新增。

- [ ] **步骤 5：最终 Commit**

```powershell
git add docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md README.md
git commit -m "docs: BlobStore 对象存储多源收尾，backlog 标记已交付"
```

- [ ] **步骤 6：合并与双仓推送（待用户拍板后执行）**

按项目既有 dual-repo 流程：合并 feature 分支到 `main` → 推 `real` → 运行 `scripts/export-public.ps1` 导出公开切片 → 在公开仓提交并推 `public`。此步骤在所有任务实现+审查完成、用户确认后进行。

---

## 自检对照

**规格覆盖：**
- §1 范围/边界/组件/数据流 → 任务 2（接口/注册表）、4（artifact 适配）、6（s3）覆盖；v0 外项（connector/skill/附件/预签名）无任务，符合。
- §2 接口/注册表/file/memory/s3/配置/artifact 适配 → 任务 1-6 一一对应。
- §3 错误处理（ErrNotFound、写失败回滚、fail-fast、ctx、无 Close）→ 任务 3/4/6 的实现与测试覆盖；回滚由任务 4 `TestPutHTMLRollsBackBlobOnMetadataFailure` 断言。
- §3 测试策略（registry/file/memory/s3 httptest/artifact 回滚/config）→ 各任务测试齐备。
- 任务 7 收尾 + 双仓推送。

**类型一致性：** `blob.Store`（`Put/Get/Delete`）、`blob.Options{File,S3}`、`blob.FileOptions{RootDir}`、`blob.S3Options{Endpoint,Region,Bucket,Prefix,AccessKey,SecretKey,UseSSL,PathStyle,AutoCreate}`、`blob.ErrNotFound`、`blob.Open/RegisterDriver/ListDrivers`、`artifact.NewStore(blobs blob.Store, backend store.SQLBackend)`、`artifact.Store.PutHTML(ctx,runID,html)/Get(ctx,id)` 在所有任务中签名一致。config 字段 `Storage.Driver/File.RootDir/S3.*` 与 `StorageUseSSL()` 一致。

**占位符：** 无 TODO/待定；每个代码步骤均含完整代码。







