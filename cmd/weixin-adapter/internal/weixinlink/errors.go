package weixinlink

import (
	"errors"
	"fmt"
)

// ErrCodeSessionTimeout is the iLink errcode that means the logged-in session
// has expired; the bot must re-scan QR to recover.
const ErrCodeSessionTimeout = -14

// SessionError is a business error returned by iLink (ret/errcode/errmsg), as
// opposed to a transport error. Callers can inspect ErrCode to decide whether
// recovery requires a fresh QR login.
type SessionError struct {
	Ret     int
	ErrCode int
	ErrMsg  string
}

func (e *SessionError) Error() string {
	return fmt.Sprintf("weixin getupdates: ret=%d errcode=%d errmsg=%s", e.Ret, e.ErrCode, e.ErrMsg)
}

// IsSessionTimeout reports whether err is an iLink session-timeout (-14) error.
// Transport failures and other errcodes return false.
func IsSessionTimeout(err error) bool {
	var se *SessionError
	if errors.As(err, &se) {
		return se.ErrCode == ErrCodeSessionTimeout
	}
	return false
}
