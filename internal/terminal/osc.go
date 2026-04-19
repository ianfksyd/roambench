package terminal

import (
	"strings"
	"time"
)

// OSCNotification represents a parsed OSC 9, 99, or 777 notification.
type OSCNotification struct {
	Code      int
	Title     string
	Subtitle  string
	Body      string
	SessionID string
	Timestamp time.Time
}

// OSCScanner is a streaming parser that extracts OSC notification sequences
// from terminal output. Non-notification bytes pass through unchanged.
//
// Supported sequences:
//   - OSC 9:   \x1b]9;<body>\x07
//   - OSC 99:  \x1b]99;<params>;<payload>\x07  (kitty protocol, simplified)
//   - OSC 777: \x1b]777;notify;<title>;<body>\x07
//
// Terminators: BEL (\x07) or ST (\x1b\\).
type OSCScanner struct {
	sessionID string
	buf       []byte
	inEsc     bool // saw \x1b, waiting for ]
	inOSC     bool // inside \x1b]..., collecting until terminator
}

// NewOSCScanner creates a scanner for the given terminal session.
func NewOSCScanner(sessionID string) *OSCScanner {
	return &OSCScanner{sessionID: sessionID}
}

// Feed processes raw terminal output bytes. It returns the passthrough bytes
// (with OSC notification sequences stripped) and any parsed notifications.
func (s *OSCScanner) Feed(data []byte) (passthrough []byte, notifications []OSCNotification) {
	for i := 0; i < len(data); i++ {
		b := data[i]

		if s.inOSC {
			if b == 0x07 { // BEL terminator
				if n, ok := s.parseOSC(); ok {
					notifications = append(notifications, n)
				}
				s.buf = s.buf[:0]
				s.inOSC = false
				continue
			}
			if b == '\\' && len(s.buf) > 0 && s.buf[len(s.buf)-1] == 0x1b { // ST = ESC \
				s.buf = s.buf[:len(s.buf)-1] // remove the ESC from buf
				if n, ok := s.parseOSC(); ok {
					notifications = append(notifications, n)
				}
				s.buf = s.buf[:0]
				s.inOSC = false
				continue
			}
			if len(s.buf) < 1024 { // cap buffer to avoid unbounded growth
				s.buf = append(s.buf, b)
			}
			continue
		}

		if s.inEsc {
			s.inEsc = false
			if b == ']' {
				s.inOSC = true
				s.buf = s.buf[:0]
				continue
			}
			// Not an OSC start — pass through the ESC and this byte
			passthrough = append(passthrough, 0x1b, b)
			continue
		}

		if b == 0x1b {
			s.inEsc = true
			continue
		}

		passthrough = append(passthrough, b)
	}
	return
}

func (s *OSCScanner) parseOSC() (OSCNotification, bool) {
	content := string(s.buf)

	// OSC 9: 9;<body>
	if strings.HasPrefix(content, "9;") {
		return OSCNotification{
			Code:      9,
			Title:     strings.TrimSpace(content[2:]),
			SessionID: s.sessionID,
			Timestamp: time.Now().UTC(),
		}, true
	}

	// OSC 777: 777;notify;<title>;<body>
	if strings.HasPrefix(content, "777;notify;") {
		rest := content[11:]
		title, body, _ := strings.Cut(rest, ";")
		return OSCNotification{
			Code:      777,
			Title:     strings.TrimSpace(title),
			Body:      strings.TrimSpace(body),
			SessionID: s.sessionID,
			Timestamp: time.Now().UTC(),
		}, true
	}

	// OSC 99: simplified kitty — look for p=body payload
	if strings.HasPrefix(content, "99;") {
		rest := content[3:]
		// Find the payload after the last ':'
		if idx := strings.LastIndex(rest, ":"); idx >= 0 {
			payload := strings.TrimSpace(rest[idx+1:])
			if payload != "" {
				return OSCNotification{
					Code:      99,
					Body:      payload,
					SessionID: s.sessionID,
					Timestamp: time.Now().UTC(),
				}, true
			}
		}
	}

	return OSCNotification{}, false
}
