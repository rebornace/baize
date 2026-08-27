package conversation

import "errors"

// ErrMessageNotFound is returned when a message id does not exist in the conversation.
var ErrMessageNotFound = errors.New("message not found")
