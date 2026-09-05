package types

import errorsmod "cosmossdk.io/errors"

// Codes start at 2; code 1 is reserved by cosmossdk.io/errors. Never renumber (SPEC §6.9).
var (
	ErrAppNotFound        = errorsmod.Register(ModuleName, 2, "app not found")
	ErrUnauthorized       = errorsmod.Register(ModuleName, 3, "signer is not the app owner")
	ErrInvalidField       = errorsmod.Register(ModuleName, 4, "invalid field")
	ErrInvalidIcon        = errorsmod.Register(ModuleName, 5, "invalid icon")
	ErrInvalidCategory    = errorsmod.Register(ModuleName, 6, "category not allowed")
	ErrInvalidSemver      = errorsmod.Register(ModuleName, 7, "invalid semantic version")
	ErrInvalidMagnet      = errorsmod.Register(ModuleName, 8, "invalid magnet link")
	ErrInvalidChecksum    = errorsmod.Register(ModuleName, 9, "invalid sha256 checksum")
	ErrVersionExists      = errorsmod.Register(ModuleName, 10, "version already exists")
	ErrVersionNotFound    = errorsmod.Register(ModuleName, 11, "version not found")
	ErrVersionYanked      = errorsmod.Register(ModuleName, 12, "version is yanked")
	ErrAlreadyVerified    = errorsmod.Register(ModuleName, 13, "version already has a blue check")
	ErrNotVerified        = errorsmod.Register(ModuleName, 14, "version has no blue check")
	ErrRequestExists      = errorsmod.Register(ModuleName, 15, "an open request already exists for this version")
	ErrRequestNotFound    = errorsmod.Register(ModuleName, 16, "request not found")
	ErrRequestNotOpen     = errorsmod.Register(ModuleName, 17, "request is not open")
	ErrNotBondedValidator = errorsmod.Register(ModuleName, 18, "signer is not a bonded validator")
	ErrInvalidEscrow      = errorsmod.Register(ModuleName, 19, "invalid escrow coin")
	ErrInvalidParams      = errorsmod.Register(ModuleName, 20, "invalid params")
)
