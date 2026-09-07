package webhook

import "context"

// adminClient talks to the out-of-process adapter's management plane
// (/admin/*). The HTTP implementation is added in a later task; tests inject
// fakes. All methods return an error when the adapter is unreachable.
type adminClient interface {
	// Status returns hasCredentials and polling from the adapter /admin/status.
	Status(ctx context.Context) (hasCreds bool, polling bool, err error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	LoginStart(ctx context.Context) (ticket, qrURL string, err error)
	// LoginPoll polls QR login; status is "pending"|"success"|"expired".
	LoginPoll(ctx context.Context, ticket string) (status string, err error)
	Logout(ctx context.Context) error
}
