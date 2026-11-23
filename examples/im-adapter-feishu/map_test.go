package main

import "testing"

func TestMapFeishuEvent(t *testing.T) {
	raw := []byte(`{"schema":"2.0","header":{"event_id":"ev1","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_alice"}},"message":{"message_type":"text","content":"{\"text\":\"你好\"}"}}}`)
	peer, text, idem, err := mapFeishuEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if peer != "ou_alice" || text != "你好" || idem != "ev1" {
		t.Fatalf("peer=%q text=%q idem=%q", peer, text, idem)
	}
}
