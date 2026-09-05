package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/legacy"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers the module's sdk.Msg implementations and the Msg service.
func RegisterInterfaces(ir codectypes.InterfaceRegistry) {
	ir.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateApp{},
		&MsgUpdateApp{},
		&MsgTransferApp{},
		&MsgSetDeprecated{},
		&MsgPublishVersion{},
		&MsgYankVersion{},
		&MsgRequestBlueCheck{},
		&MsgRequestRevocation{},
		&MsgVote{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(ir, &_Msg_serviceDesc)
}

// RegisterLegacyAminoCodec registers amino names (must match amino.name options in tx.proto).
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	legacy.RegisterAminoMsg(cdc, &MsgCreateApp{}, "registry/MsgCreateApp")
	legacy.RegisterAminoMsg(cdc, &MsgUpdateApp{}, "registry/MsgUpdateApp")
	legacy.RegisterAminoMsg(cdc, &MsgTransferApp{}, "registry/MsgTransferApp")
	legacy.RegisterAminoMsg(cdc, &MsgSetDeprecated{}, "registry/MsgSetDeprecated")
	legacy.RegisterAminoMsg(cdc, &MsgPublishVersion{}, "registry/MsgPublishVersion")
	legacy.RegisterAminoMsg(cdc, &MsgYankVersion{}, "registry/MsgYankVersion")
	legacy.RegisterAminoMsg(cdc, &MsgRequestBlueCheck{}, "registry/MsgRequestBlueCheck")
	legacy.RegisterAminoMsg(cdc, &MsgRequestRevocation{}, "registry/MsgRequestRevocation")
	legacy.RegisterAminoMsg(cdc, &MsgVote{}, "registry/MsgVote")
	legacy.RegisterAminoMsg(cdc, &MsgUpdateParams{}, "registry/MsgUpdateParams")
	cdc.RegisterConcrete(&Params{}, "registry/Params", nil)
}
