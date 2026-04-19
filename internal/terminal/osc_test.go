package terminal

import (
	"testing"
)

func TestOSCScannerPassthroughNormalData(t *testing.T) {
	s := NewOSCScanner("sess-1")
	pass, notifs := s.Feed([]byte("hello world"))
	if string(pass) != "hello world" {
		t.Fatalf("passthrough = %q, want %q", pass, "hello world")
	}
	if len(notifs) != 0 {
		t.Fatalf("notifications = %v, want none", notifs)
	}
}

func TestOSCScannerParsesOSC9WithBEL(t *testing.T) {
	s := NewOSCScanner("sess-1")
	pass, notifs := s.Feed([]byte("before\x1b]9;Build complete\x07after"))
	if string(pass) != "beforeafter" {
		t.Fatalf("passthrough = %q, want %q", pass, "beforeafter")
	}
	if len(notifs) != 1 {
		t.Fatalf("len(notifications) = %d, want 1", len(notifs))
	}
	if notifs[0].Code != 9 || notifs[0].Title != "Build complete" {
		t.Fatalf("notification = %+v, want Code=9 Title=Build complete", notifs[0])
	}
	if notifs[0].SessionID != "sess-1" {
		t.Fatalf("SessionID = %q, want sess-1", notifs[0].SessionID)
	}
}

func TestOSCScannerParsesOSC777(t *testing.T) {
	s := NewOSCScanner("sess-2")
	pass, notifs := s.Feed([]byte("\x1b]777;notify;Task Done;All tests passed\x07"))
	if string(pass) != "" {
		t.Fatalf("passthrough = %q, want empty", pass)
	}
	if len(notifs) != 1 {
		t.Fatalf("len(notifications) = %d, want 1", len(notifs))
	}
	n := notifs[0]
	if n.Code != 777 || n.Title != "Task Done" || n.Body != "All tests passed" {
		t.Fatalf("notification = %+v, want Code=777 Title=Task Done Body=All tests passed", n)
	}
}

func TestOSCScannerParsesOSC99Kitty(t *testing.T) {
	s := NewOSCScanner("sess-3")
	pass, notifs := s.Feed([]byte("\x1b]99;i=1;e=1;d=0:Hello World\x07"))
	if string(pass) != "" {
		t.Fatalf("passthrough = %q, want empty", pass)
	}
	if len(notifs) != 1 {
		t.Fatalf("len(notifications) = %d, want 1", len(notifs))
	}
	if notifs[0].Code != 99 || notifs[0].Body != "Hello World" {
		t.Fatalf("notification = %+v, want Code=99 Body=Hello World", notifs[0])
	}
}

func TestOSCScannerSTTerminator(t *testing.T) {
	s := NewOSCScanner("sess-1")
	pass, notifs := s.Feed([]byte("\x1b]9;Done\x1b\\rest"))
	if string(pass) != "rest" {
		t.Fatalf("passthrough = %q, want %q", pass, "rest")
	}
	if len(notifs) != 1 || notifs[0].Title != "Done" {
		t.Fatalf("notifications = %+v, want [Done]", notifs)
	}
}

func TestOSCScannerPartialSequenceAcrossFeeds(t *testing.T) {
	s := NewOSCScanner("sess-1")
	// Split the sequence across two Feed calls
	pass1, notifs1 := s.Feed([]byte("data\x1b]9;Build"))
	if string(pass1) != "data" {
		t.Fatalf("pass1 = %q, want %q", pass1, "data")
	}
	if len(notifs1) != 0 {
		t.Fatalf("notifs1 = %v, want none", notifs1)
	}

	pass2, notifs2 := s.Feed([]byte(" complete\x07more"))
	if string(pass2) != "more" {
		t.Fatalf("pass2 = %q, want %q", pass2, "more")
	}
	if len(notifs2) != 1 || notifs2[0].Title != "Build complete" {
		t.Fatalf("notifs2 = %+v, want [Build complete]", notifs2)
	}
}

func TestOSCScannerNonOSCEscapePassesThrough(t *testing.T) {
	s := NewOSCScanner("sess-1")
	// ESC [ is CSI, not OSC — should pass through
	pass, notifs := s.Feed([]byte("\x1b[31mred\x1b[0m"))
	if string(pass) != "\x1b[31mred\x1b[0m" {
		t.Fatalf("passthrough = %q, want CSI sequence preserved", pass)
	}
	if len(notifs) != 0 {
		t.Fatalf("notifications = %v, want none", notifs)
	}
}

func TestOSCScannerMultipleNotificationsInOneFeed(t *testing.T) {
	s := NewOSCScanner("sess-1")
	data := []byte("\x1b]9;First\x07middle\x1b]777;notify;Second;Body2\x07end")
	pass, notifs := s.Feed(data)
	if string(pass) != "middleend" {
		t.Fatalf("passthrough = %q, want %q", pass, "middleend")
	}
	if len(notifs) != 2 {
		t.Fatalf("len(notifications) = %d, want 2", len(notifs))
	}
	if notifs[0].Code != 9 || notifs[0].Title != "First" {
		t.Fatalf("notifs[0] = %+v, want Code=9 Title=First", notifs[0])
	}
	if notifs[1].Code != 777 || notifs[1].Title != "Second" || notifs[1].Body != "Body2" {
		t.Fatalf("notifs[1] = %+v, want Code=777 Title=Second Body=Body2", notifs[1])
	}
}

func TestOSCScannerIgnoresUnknownOSC(t *testing.T) {
	s := NewOSCScanner("sess-1")
	// OSC 0 (set window title) should be stripped but not produce a notification
	pass, notifs := s.Feed([]byte("a\x1b]0;my title\x07b"))
	if string(pass) != "ab" {
		t.Fatalf("passthrough = %q, want %q", pass, "ab")
	}
	if len(notifs) != 0 {
		t.Fatalf("notifications = %v, want none (OSC 0 is not a notification)", notifs)
	}
}
