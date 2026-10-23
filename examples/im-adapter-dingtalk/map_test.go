package main

import "testing"

func TestMapDingTalkCallback(t *testing.T) {
	raw := []byte(`{"msgtype":"text","text":{"content":"你好"},"senderStaffId":"staff_alice","msgId":"msg-1"}`)
	peer, text, idem, err := mapDingTalkCallback(raw)
	if err != nil {
		t.Fatal(err)
	}
	if peer != "staff_alice" || text != "你好" || idem != "msg-1" {
		t.Fatalf("peer=%q text=%q idem=%q", peer, text, idem)
	}
}