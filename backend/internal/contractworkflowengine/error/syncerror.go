// Package error holds sentinel errors specific to the contract workflow
// engine's cross-peer synchronization (see remotesync, dcstodcs).
package error

import "errors"

var ErrOutdatedContractData = errors.New("contract was updated elsewhere, please force synchronisation")
