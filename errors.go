//go:build !wasm

package goflare

import "errors"

var (
	ErrBrowserTagForbidden = errors.New("goflare: the Cloudflare Worker cannot be compiled with -tags browser")

	ErrD1NotFound             = errors.New("ErrD1NotFound")
	ErrBindingWithoutName     = errors.New("ErrBindingWithoutName")
	ErrOneDatabaseOneBucket   = errors.New("ErrOneDatabaseOneBucket")
	ErrUnknownPermissionGroup = errors.New("ErrUnknownPermissionGroup")
	ErrNoPersonalKeyring      = errors.New("ErrNoPersonalKeyring")
	ErrNoRootDir              = errors.New("ErrNoRootDir")
	ErrInvalidVarLine         = errors.New("ErrInvalidVarLine")
	ErrNoBootstrapToken       = errors.New("ErrNoBootstrapToken")
	ErrNotOneAccount          = errors.New("ErrNotOneAccount")
	ErrBootstrapTokenInactive = errors.New("ErrBootstrapTokenInactive")
	ErrZoneNotFound           = errors.New("ErrZoneNotFound")
	ErrNoD1Name               = errors.New("ErrNoD1Name")
)
