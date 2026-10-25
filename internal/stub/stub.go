package stub

import (
	"errors"
	"fmt"
)

func Err(op string) error {
	return fmt.Errorf("%s: not implemented: %w", op, errors.ErrUnsupported)
}
