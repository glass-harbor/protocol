package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// getVersion loads a version or returns ErrVersionNotFound.
func (k Keeper) getVersion(ctx context.Context, appID uint64, version string) (types.Version, error) {
	v, err := k.Versions.Get(ctx, collections.Join(appID, version))
	if errors.Is(err, collections.ErrNotFound) {
		return types.Version{}, errorsmod.Wrapf(types.ErrVersionNotFound, "%d/%s", appID, version)
	}
	return v, err
}
