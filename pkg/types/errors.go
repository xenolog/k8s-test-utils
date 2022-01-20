package types

import (
	"errors"
	"fmt"
)

var (
	Error                  = errors.New("")
	ErrorUndefined         = fmt.Errorf("not defined%w", Error)
	ErrorClusterUndefined  = fmt.Errorf("cluster %w", ErrorUndefined)
	ErrorRegionUndefined   = fmt.Errorf("region %w", ErrorUndefined)
	ErrorProviderUndefined = fmt.Errorf("provider %w", ErrorUndefined)

	ErrorObjLookup          = fmt.Errorf("%w", Error)
	ErrorNotFound           = fmt.Errorf("not found%w", ErrorObjLookup)
	ErrorMoreThanOne        = fmt.Errorf("more than one%w", ErrorObjLookup)
	ErrorInoperable         = fmt.Errorf("inoperable%w", Error)
	ErrorWrong              = fmt.Errorf("wrong%w", Error)
	ErrorWrongFormat        = fmt.Errorf("%w format", ErrorWrong)
	ErrorWrongObject        = fmt.Errorf("%w object", ErrorWrong)
	ErrorWrongName          = fmt.Errorf("%w name", ErrorWrong)
	ErrorWrongParametr      = fmt.Errorf("%w parameter", ErrorWrong)
	ErrorWrongRequest       = fmt.Errorf("%w request", ErrorWrong)
	ErrorTimeoutOrCancel    = fmt.Errorf("timeout or cancel%w", Error)
	ErrorUnableToAllocate   = fmt.Errorf("unable to allocate%w", Error)
	ErrorDoNothing          = fmt.Errorf("do nothing%w", Error)
	ErrorSomethingWentWrong = fmt.Errorf("Something went wrong%w", Error)
)
