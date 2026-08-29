package conversation

import "errors"

// ErrMessageNotFound is returned when a message id does not exist in the conversation.
var ErrMessageNotFound = errors.New("message not found")

// ErrMetaNotFound is returned by GetMeta when no conversation_meta row exists.
var ErrMetaNotFound = errors.New("conversation meta not found")
