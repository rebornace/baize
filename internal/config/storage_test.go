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
		s.UseSSL == nil || *s.UseSSL || !s.PathStyle || !s.AutoCreateBucket {
		t.Fatalf("s3 config not parsed: %+v", s)
	}
	if cfg.StorageUseSSL() {
		t.Fatalf("use_ssl=false should be respected")
	}
}
