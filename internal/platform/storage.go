package platform

import (
	"context"
	"io"
)

// ObjectStorage is the server-owned storage boundary. Intake and filesystem
// promotion are intentionally deferred to the upload stage.
type ObjectStorage interface {
	Put(context.Context, string, io.Reader, int64) error
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
