package error

import "errors"

var ErrOutdatedContractData = errors.New("contract was updated elsewhere, please force synchronisation")
