package registry

import (
	"fmt"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"github.com/cosmos/cosmos-sdk/version"
)

const (
	queryService = "glassharbor.registry.v1.Query"
	msgService   = "glassharbor.registry.v1.Msg"
)

// AutoCLIOptions describes the CLI (SPEC §8).
func (AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: queryService,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query registry params"},
				{RpcMethod: "App", Use: "app <app-id>", Short: "Query an app by id", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Apps", Use: "apps", Short: "List apps, optionally by --owner"},
				{RpcMethod: "Versions", Use: "versions <app-id>", Short: "List versions of an app, newest first", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}}},
				{RpcMethod: "Version", Use: "version <app-id> <version>", Short: "Query one version", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}}},
				{RpcMethod: "Request", Use: "request <request-id>", Short: "Query a blue-check request", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Requests", Use: "requests", Short: "List requests, optionally by --status"},
				{RpcMethod: "Votes", Use: "votes <request-id>", Short: "List votes on a request", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "request_id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: msgService,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "CreateApp", Use: "create-app", Short: "Register a new app (charges create_app_fee)",
					Example: fmt.Sprintf(`%s tx registry create-app --title "Jetty Wallet" --category wallet --icon "$(base64 < icon.png)" --icon-mime image/png --from alice`, version.AppName),
				},
				{
					RpcMethod: "UpdateApp", Use: "update-app <app-id>", Short: "Replace all editable app metadata",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}},
				},
				{
					RpcMethod: "TransferApp", Use: "transfer-app <app-id> <new-owner>", Short: "Transfer ownership immediately",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "new_owner"}},
				},
				{
					RpcMethod: "SetDeprecated", Use: "set-deprecated <app-id> <true|false>", Short: "Toggle the deprecated flag",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "deprecated"}},
				},
				{
					RpcMethod: "PublishVersion", Use: "publish-version <app-id> <version> <magnet> <sha256> <file-size>", Short: "Publish an immutable version (charges publish_version_fee)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}, {ProtoField: "magnet"}, {ProtoField: "checksum_sha256"}, {ProtoField: "file_size"}},
				},
				{
					RpcMethod: "YankVersion", Use: "yank-version <app-id> <version>", Short: "Irreversibly mark a version unsafe",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}},
				},
				{
					RpcMethod: "RequestBlueCheck", Use: "request-blue-check <app-id> <version>", Short: "Open a verification vote; add --escrow 1000000uglass to reward validators",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}},
				},
				{
					RpcMethod: "RequestRevocation", Use: "request-revocation <app-id> <version>", Short: "Open a revocation vote (validator operator key)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}},
				},
				{
					RpcMethod: "Vote", Use: "vote <request-id> <yes|no>", Short: "Vote on a request (validator operator key)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "request_id"}, {ProtoField: "option"}},
				},
				{
					RpcMethod: "UpdateParams", Use: "update-params-proposal <params>", Short: "Submit a gov proposal to replace registry params (whole params JSON)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}}, GovProposal: true,
				},
			},
		},
	}
}
