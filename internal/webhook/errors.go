package webhook

import "errors"

var (
	ErrNoURL          = errors.New("webhook url is not configured")
	ErrDeliveryFailed = errors.New("webhook delivery failed")
)
