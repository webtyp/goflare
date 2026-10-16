//go:build !wasm

package goflare

import "errors"

var (
	ErrD1NotFound             = errors.New("ErrD1NotFound")
	ErrBindingWithoutName     = errors.New("ErrBindingWithoutName")
	ErrOneDatabaseOneBucket   = errors.New("ErrOneDatabaseOneBucket")
	ErrUnknownPermissionGroup = errors.New("ErrUnknownPermissionGroup")
	ErrNoPersonalKeyring      = errors.New("ErrNoPersonalKeyring")
	ErrNoRootDir              = errors.New("ErrNoRootDir")
	ErrInvalidVarLine         = errors.New("ErrInvalidVarLine")
)
