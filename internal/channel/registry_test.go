package channel

import (
	"context"
	"testing"
)

// stubChannel is a minimal Channel used by the registry Descriptor tests.
type stubChannel struct {
	name string
}

func (s *stubChannel) Name() string { return s.name }

func (s *stubChannel) Start(context.Context) error { return nil }

func (s *stubChannel) Stop(context.Context) error { return nil }

func (s *stubChannel) SendText(context.Context, string, string, map[string]string) error {
	return nil
}

func (s *stubChannel) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}

func TestRegisterAndDescribe(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(Descriptor{
		Name:            "stub",
		Build:           func(Config) (Channel, error) { return &stubChannel{name: "stub"}, nil },
		DefaultCredsDir: "data/channels/stub",
	})

	d, ok := Describe("stub")
	if !ok {
		t.Fatal("Describe: stub not found")
	}
	if d.Name != "stub" || d.DefaultCredsDir != "data/channels/stub" || d.Build == nil {
		t.Fatalf("Describe descriptor=%+v", d)
	}
	if _, ok := Describe("missing"); ok {
		t.Fatal("Describe: unknown channel should report ok=false")
	}

	ch, err := Open("stub", nil)
	if err != nil || ch.Name() != "stub" {
		t.Fatalf("Open ch=%v err=%v", ch, err)
	}
	names := Descriptors()
	if len(names) != 1 || names[0].Name != "stub" {
		t.Fatalf("Descriptors=%+v", names)
	}
}

func TestRegisterChannelBackCompat(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	RegisterChannel("legacy", func(Config) (Channel, error) { return &stubChannel{name: "legacy"}, nil })
	if _, ok := Describe("legacy"); !ok {
		t.Fatal("RegisterChannel should also be discoverable via Describe")
	}
	if got := List(); len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("List=%v", got)
	}
}
