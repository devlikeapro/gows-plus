package storage

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

var ErrStorageDisabled = errors.New("storage disabled")

func StorageDisabled(name string) error {
	return fmt.Errorf("%w: %s", ErrStorageDisabled, name)
}
