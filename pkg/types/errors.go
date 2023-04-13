package types

import (
	"errors"
	"fmt"
)

var (
	Err                  = errors.New("")
	ErrUndefined         = fmt.Errorf("not defined%w", Err)
	ErrClusterUndefined  = fmt.Errorf("cluster %w", ErrUndefined)
	ErrRegionUndefined   = fmt.Errorf("region %w", ErrUndefined)
	ErrProviderUndefined = fmt.Errorf("provider %w", ErrUndefined)

	ErrObjLookup          = fmt.Errorf("%w", Err)
	ErrNotFound           = fmt.Errorf("not found%w", ErrObjLookup)
	ErrMoreThanOne        = fmt.Errorf("more than one%w", ErrObjLookup)
	ErrInoperable         = fmt.Errorf("inoperable%w", Err)
	ErrWrong              = fmt.Errorf("wrong%w", Err)
	ErrWrongFormat        = fmt.Errorf("%w format", ErrWrong)
	ErrWrongObject        = fmt.Errorf("%w object", ErrWrong)
	ErrWrongName          = fmt.Errorf("%w name", ErrWrong)
	ErrWrongParametr      = fmt.Errorf("%w parameter", ErrWrong)
	ErrWrongRequest       = fmt.Errorf("%w request", ErrWrong)
	ErrTimeoutOrCancel    = fmt.Errorf("timeout or cancel%w", Err)
	ErrUnableToAllocate   = fmt.Errorf("unable to allocate%w", Err)
	ErrDoNothing          = fmt.Errorf("do nothing%w", Err)
	ErrSomethingWentWrong = fmt.Errorf("Something went wrong%w", Err)
)
