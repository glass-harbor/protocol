package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/gogoproto/proto"
	"github.com/cosmos/ibc-go/v11/modules/apps/transfer"
	ibctransferkeeper "github.com/cosmos/ibc-go/v11/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v11/modules/apps/transfer/types"
	transferv2 "github.com/cosmos/ibc-go/v11/modules/apps/transfer/v2"
	ibc "github.com/cosmos/ibc-go/v11/modules/core"
	porttypes "github.com/cosmos/ibc-go/v11/modules/core/05-port/types"
	ibcapi "github.com/cosmos/ibc-go/v11/modules/core/api"
	ibcexported "github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v11/modules/core/keeper"
	ibctm "github.com/cosmos/ibc-go/v11/modules/light-clients/07-tendermint"
	"github.com/spf13/cast"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	reflectionv1 "cosmossdk.io/api/cosmos/reflection/v1"
	"cosmossdk.io/client/v2/autocli"
	clienthelpers "cosmossdk.io/client/v2/helpers"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	runtimeservices "github.com/cosmos/cosmos-sdk/runtime/services"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/std"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	"github.com/cosmos/cosmos-sdk/x/auth/posthandler"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	consensusparamkeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	evidencekeeper "github.com/cosmos/cosmos-sdk/x/evidence/keeper"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	feegrantkeeper "github.com/cosmos/cosmos-sdk/x/feegrant/keeper"
	feegrantmodule "github.com/cosmos/cosmos-sdk/x/feegrant/module"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/cosmos-sdk/x/mint"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/protocolpool"
	protocolpoolkeeper "github.com/cosmos/cosmos-sdk/x/protocolpool/keeper"
	protocolpooltypes "github.com/cosmos/cosmos-sdk/x/protocolpool/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/cosmos-sdk/x/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradekeeper "github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	"github.com/glass-harbor/protocol/docs"
	"github.com/glass-harbor/protocol/x/registry"
	registrykeeper "github.com/glass-harbor/protocol/x/registry/keeper"
	registrytypes "github.com/glass-harbor/protocol/x/registry/types"
)

var (
	// DefaultNodeHome is the default home directory (~/.harbord).
	DefaultNodeHome string

	maccPerms = map[string][]string{
		authtypes.FeeCollectorName:                  nil,
		distrtypes.ModuleName:                       nil,
		minttypes.ModuleName:                        {authtypes.Minter},
		stakingtypes.BondedPoolName:                 {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName:              {authtypes.Burner, authtypes.Staking},
		govtypes.ModuleName:                         {authtypes.Burner},
		protocolpooltypes.ModuleName:                nil,
		protocolpooltypes.ProtocolPoolEscrowAccount: nil,
		ibctransfertypes.ModuleName:                 {authtypes.Minter, authtypes.Burner},
		registrytypes.ModuleName:                    nil,
	}
)

var (
	_ runtime.AppI            = (*App)(nil)
	_ servertypes.Application = (*App)(nil)
)

// App is the Glass Harbor Protocol application.
type App struct {
	*baseapp.BaseApp
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry types.InterfaceRegistry

	keys map[string]*storetypes.KVStoreKey

	AccountKeeper         authkeeper.AccountKeeper
	BankKeeper            bankkeeper.BaseKeeper
	StakingKeeper         *stakingkeeper.Keeper
	SlashingKeeper        slashingkeeper.Keeper
	MintKeeper            mintkeeper.Keeper
	DistrKeeper           distrkeeper.Keeper
	GovKeeper             govkeeper.Keeper
	UpgradeKeeper         *upgradekeeper.Keeper
	EvidenceKeeper        evidencekeeper.Keeper
	ConsensusParamsKeeper consensusparamkeeper.Keeper
	FeeGrantKeeper        feegrantkeeper.Keeper
	AuthzKeeper           authzkeeper.Keeper
	ProtocolPoolKeeper    protocolpoolkeeper.Keeper
	IBCKeeper             *ibckeeper.Keeper
	TransferKeeper        *ibctransferkeeper.Keeper
	RegistryKeeper        registrykeeper.Keeper

	ModuleManager      *module.Manager
	BasicModuleManager module.BasicManager
	configurator       module.Configurator
}

func init() {
	SetAddressPrefixes()
	var err error
	DefaultNodeHome, err = clienthelpers.GetNodeHomeDirectory(".harbord")
	if err != nil {
		panic(err)
	}
}

// NewApp constructs the application.
func NewApp(
	logger log.Logger,
	db dbm.DB,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	SetAddressPrefixes()
	interfaceRegistry, _ := types.NewInterfaceRegistryWithOptions(types.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec:          address.Bech32Codec{Bech32Prefix: sdk.GetConfig().GetBech32AccountAddrPrefix()},
			ValidatorAddressCodec: address.Bech32Codec{Bech32Prefix: sdk.GetConfig().GetBech32ValidatorAddrPrefix()},
		},
	})
	appCodec := codec.NewProtoCodec(interfaceRegistry)
	legacyAmino := codec.NewLegacyAmino()
	txConfig := authtx.NewTxConfig(appCodec, authtx.DefaultSignModes)

	if err := interfaceRegistry.SigningContext().Validate(); err != nil {
		panic(err)
	}
	std.RegisterLegacyAminoCodec(legacyAmino)
	std.RegisterInterfaces(interfaceRegistry)

	bApp := baseapp.NewBaseApp(Name, logger, db, txConfig.TxDecoder(), baseAppOptions...)
	bApp.SetVersion(version.Version)
	bApp.SetInterfaceRegistry(interfaceRegistry)
	bApp.SetTxEncoder(txConfig.TxEncoder())

	keys := storetypes.NewKVStoreKeys(
		authtypes.StoreKey, banktypes.StoreKey, stakingtypes.StoreKey, minttypes.StoreKey, distrtypes.StoreKey,
		slashingtypes.StoreKey, govtypes.StoreKey, consensusparamtypes.StoreKey, upgradetypes.StoreKey,
		feegrant.StoreKey, evidencetypes.StoreKey, authzkeeper.StoreKey, protocolpooltypes.StoreKey,
		ibcexported.StoreKey, ibctransfertypes.StoreKey, registrytypes.StoreKey,
	)
	if err := bApp.RegisterStreamingServices(appOpts, keys); err != nil {
		panic(err)
	}

	app := &App{
		BaseApp: bApp, legacyAmino: legacyAmino, appCodec: appCodec, txConfig: txConfig,
		interfaceRegistry: interfaceRegistry, keys: keys,
	}
	govAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	app.ConsensusParamsKeeper = consensusparamkeeper.NewKeeper(appCodec, runtime.NewKVStoreService(keys[consensusparamtypes.StoreKey]), govAuthority, runtime.EventService{})
	bApp.SetParamStore(app.ConsensusParamsKeeper.ParamsStore)

	app.AccountKeeper = authkeeper.NewAccountKeeper(
		appCodec, runtime.NewKVStoreService(keys[authtypes.StoreKey]), authtypes.ProtoBaseAccount, maccPerms,
		authcodec.NewBech32Codec(Bech32Prefix), Bech32Prefix, govAuthority,
		authkeeper.WithUnorderedTransactions(true),
	)
	app.BankKeeper = bankkeeper.NewBaseKeeper(
		appCodec, runtime.NewKVStoreService(keys[banktypes.StoreKey]), app.AccountKeeper, BlockedAddresses(), govAuthority, logger,
	)
	app.StakingKeeper = stakingkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[stakingtypes.StoreKey]), app.AccountKeeper, app.BankKeeper, govAuthority,
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	)
	app.MintKeeper = mintkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[minttypes.StoreKey]), app.StakingKeeper, app.AccountKeeper, app.BankKeeper,
		authtypes.FeeCollectorName, govAuthority,
	)
	app.ProtocolPoolKeeper = protocolpoolkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[protocolpooltypes.StoreKey]), app.AccountKeeper, app.BankKeeper, govAuthority,
	)
	app.DistrKeeper = distrkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[distrtypes.StoreKey]), app.AccountKeeper, app.BankKeeper, app.StakingKeeper,
		authtypes.FeeCollectorName, govAuthority, distrkeeper.WithExternalCommunityPool(app.ProtocolPoolKeeper),
	)
	app.SlashingKeeper = slashingkeeper.NewKeeper(
		appCodec, legacyAmino, runtime.NewKVStoreService(keys[slashingtypes.StoreKey]), app.StakingKeeper, govAuthority,
	)
	app.FeeGrantKeeper = feegrantkeeper.NewKeeper(appCodec, runtime.NewKVStoreService(keys[feegrant.StoreKey]), app.AccountKeeper)
	app.StakingKeeper.SetHooks(stakingtypes.NewMultiStakingHooks(app.DistrKeeper.Hooks(), app.SlashingKeeper.Hooks()))
	app.AuthzKeeper = authzkeeper.NewKeeper(runtime.NewKVStoreService(keys[authzkeeper.StoreKey]), appCodec, app.MsgServiceRouter(), app.AccountKeeper)

	skipUpgradeHeights := map[int64]bool{}
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}
	homePath := cast.ToString(appOpts.Get(flags.FlagHome))
	app.UpgradeKeeper = upgradekeeper.NewKeeper(skipUpgradeHeights, runtime.NewKVStoreService(keys[upgradetypes.StoreKey]), appCodec, homePath, app.BaseApp, govAuthority)

	app.IBCKeeper = ibckeeper.NewKeeper(appCodec, runtime.NewKVStoreService(keys[ibcexported.StoreKey]), app.UpgradeKeeper, govAuthority)

	govRouter := govv1beta1.NewRouter()
	govRouter.AddRoute(govtypes.RouterKey, govv1beta1.ProposalHandler)
	govKeeper := govkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[govtypes.StoreKey]), app.AccountKeeper, app.BankKeeper, app.DistrKeeper,
		app.MsgServiceRouter(), govtypes.DefaultConfig(), govAuthority,
		govkeeper.NewDefaultCalculateVoteResultsAndVotingPower(app.StakingKeeper),
	)
	govKeeper.SetLegacyRouter(govRouter)
	app.GovKeeper = *govKeeper.SetHooks(govtypes.NewMultiGovHooks())

	evidenceKeeper := evidencekeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[evidencetypes.StoreKey]), app.StakingKeeper, app.SlashingKeeper,
		app.AccountKeeper.AddressCodec(), runtime.ProvideCometInfoService(),
	)
	app.EvidenceKeeper = *evidenceKeeper

	// IBC transfer (no middleware) on both the classic and v2 routers
	app.TransferKeeper = ibctransferkeeper.NewKeeper(
		appCodec, app.AccountKeeper.AddressCodec(), runtime.NewKVStoreService(keys[ibctransfertypes.StoreKey]),
		app.IBCKeeper.ChannelKeeper, app.MsgServiceRouter(), app.AccountKeeper, app.BankKeeper, govAuthority,
	)
	ibcRouter := porttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, transfer.NewIBCModule(app.TransferKeeper))
	app.IBCKeeper.SetRouter(ibcRouter)
	ibcRouterV2 := ibcapi.NewRouter()
	ibcRouterV2.AddRoute(ibctransfertypes.PortID, transferv2.NewIBCModule(app.TransferKeeper))
	app.IBCKeeper.SetRouterV2(ibcRouterV2)

	tmLightClientModule := ibctm.NewLightClientModule(appCodec, app.IBCKeeper.ClientKeeper.GetStoreProvider())
	app.IBCKeeper.ClientKeeper.AddRoute(ibctm.ModuleName, &tmLightClientModule)

	app.RegistryKeeper = registrykeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[registrytypes.StoreKey]), app.AccountKeeper, app.BankKeeper, app.StakingKeeper, govAuthority,
	)

	app.ModuleManager = module.NewManager(
		genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app, txConfig),
		auth.NewAppModule(appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts, nil),
		vesting.NewAppModule(app.AccountKeeper, app.BankKeeper),
		bank.NewAppModule(appCodec, app.BankKeeper, app.AccountKeeper, nil),
		feegrantmodule.NewAppModule(appCodec, app.AccountKeeper, app.BankKeeper, app.FeeGrantKeeper, app.interfaceRegistry),
		gov.NewAppModule(appCodec, &app.GovKeeper, app.AccountKeeper, app.BankKeeper, nil),
		mint.NewAppModule(appCodec, app.MintKeeper, app.AccountKeeper, nil, nil),
		slashing.NewAppModule(appCodec, app.SlashingKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, nil, app.interfaceRegistry),
		distr.NewAppModule(appCodec, app.DistrKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, nil),
		staking.NewAppModule(appCodec, app.StakingKeeper, app.AccountKeeper, app.BankKeeper, nil),
		upgrade.NewAppModule(app.UpgradeKeeper, app.AccountKeeper.AddressCodec()),
		evidence.NewAppModule(app.EvidenceKeeper),
		authzmodule.NewAppModule(appCodec, app.AuthzKeeper, app.AccountKeeper, app.BankKeeper, app.interfaceRegistry),
		consensus.NewAppModule(appCodec, app.ConsensusParamsKeeper),
		protocolpool.NewAppModule(app.ProtocolPoolKeeper, app.AccountKeeper, app.BankKeeper),
		ibc.NewAppModule(app.IBCKeeper),
		transfer.NewAppModule(app.TransferKeeper),
		ibctm.NewAppModule(tmLightClientModule),
		registry.NewAppModule(app.RegistryKeeper),
	)
	app.BasicModuleManager = module.NewBasicManagerFromManager(app.ModuleManager, map[string]module.AppModuleBasic{
		genutiltypes.ModuleName: genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
		govtypes.ModuleName:     gov.NewAppModuleBasic([]govclient.ProposalHandler{}),
	})
	// the native denom metadata must reach `harbord init`, which builds genesis from the
	// basic manager and never sees App.DefaultGenesis (SPEC §4.1)
	for _, name := range []string{banktypes.ModuleName, govtypes.ModuleName} {
		app.BasicModuleManager[name] = chainGenesisDefaults{app.BasicModuleManager[name]}
	}
	app.BasicModuleManager.RegisterLegacyAminoCodec(legacyAmino)
	app.BasicModuleManager.RegisterInterfaces(interfaceRegistry)

	app.ModuleManager.SetOrderPreBlockers(upgradetypes.ModuleName, authtypes.ModuleName)
	app.ModuleManager.SetOrderBeginBlockers(
		minttypes.ModuleName, distrtypes.ModuleName, protocolpooltypes.ModuleName, slashingtypes.ModuleName,
		evidencetypes.ModuleName, stakingtypes.ModuleName, ibcexported.ModuleName, ibctransfertypes.ModuleName,
		genutiltypes.ModuleName, authz.ModuleName,
	)
	// registry runs after staking so bonded power is final for the block (SPEC §4.3)
	app.ModuleManager.SetOrderEndBlockers(
		banktypes.ModuleName, govtypes.ModuleName, stakingtypes.ModuleName, registrytypes.ModuleName,
		ibcexported.ModuleName, ibctransfertypes.ModuleName, genutiltypes.ModuleName, feegrant.ModuleName,
		protocolpooltypes.ModuleName,
	)
	genesisModuleOrder := []string{
		authtypes.ModuleName, banktypes.ModuleName, distrtypes.ModuleName, stakingtypes.ModuleName,
		slashingtypes.ModuleName, govtypes.ModuleName, minttypes.ModuleName, ibcexported.ModuleName,
		genutiltypes.ModuleName, evidencetypes.ModuleName, authz.ModuleName, feegrant.ModuleName,
		ibctransfertypes.ModuleName, upgradetypes.ModuleName, vestingtypes.ModuleName,
		consensusparamtypes.ModuleName, protocolpooltypes.ModuleName, registrytypes.ModuleName,
	}
	exportModuleOrder := []string{
		consensusparamtypes.ModuleName, authtypes.ModuleName, protocolpooltypes.ModuleName, banktypes.ModuleName,
		distrtypes.ModuleName, stakingtypes.ModuleName, slashingtypes.ModuleName, govtypes.ModuleName,
		minttypes.ModuleName, ibcexported.ModuleName, genutiltypes.ModuleName, evidencetypes.ModuleName,
		authz.ModuleName, feegrant.ModuleName, ibctransfertypes.ModuleName, upgradetypes.ModuleName,
		vestingtypes.ModuleName, registrytypes.ModuleName,
	}
	app.ModuleManager.SetOrderInitGenesis(genesisModuleOrder...)
	app.ModuleManager.SetOrderExportGenesis(exportModuleOrder...)

	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())
	if err := app.ModuleManager.RegisterServices(app.configurator); err != nil {
		panic(err)
	}

	autocliv1.RegisterQueryServer(app.GRPCQueryRouter(), runtimeservices.NewAutoCLIQueryService(app.ModuleManager.Modules))
	reflectionSvc, err := runtimeservices.NewReflectionService()
	if err != nil {
		panic(err)
	}
	reflectionv1.RegisterReflectionServiceServer(app.GRPCQueryRouter(), reflectionSvc)

	app.MountKVStores(keys)
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PreBlocker)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetEndBlocker(app.EndBlocker)
	app.setAnteHandler(txConfig)
	app.setPostHandler()

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			panic(fmt.Errorf("error loading last version: %w", err))
		}
	}
	return app
}

func (app *App) setAnteHandler(txConfig client.TxConfig) {
	anteHandler, err := NewAnteHandler(HandlerOptions{
		HandlerOptions: ante.HandlerOptions{
			AccountKeeper:   app.AccountKeeper,
			BankKeeper:      app.BankKeeper,
			SignModeHandler: txConfig.SignModeHandler(),
			FeegrantKeeper:  app.FeeGrantKeeper,
			SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
			SigVerifyOptions: []ante.SigVerificationDecoratorOption{
				ante.WithUnorderedTxGasCost(ante.DefaultUnorderedTxGasCost),
				ante.WithMaxUnorderedTxTimeoutDuration(ante.DefaultMaxTimeoutDuration),
			},
		},
		IBCKeeper: app.IBCKeeper,
	})
	if err != nil {
		panic(err)
	}
	app.SetAnteHandler(anteHandler)
}

func (app *App) setPostHandler() {
	postHandler, err := posthandler.NewPostHandler(posthandler.HandlerOptions{})
	if err != nil {
		panic(err)
	}
	app.SetPostHandler(postHandler)
}

// Name returns the app name.
func (app *App) Name() string { return app.BaseApp.Name() }

// PreBlocker runs module pre-blockers.
func (app *App) PreBlocker(ctx sdk.Context, _ *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
	return app.ModuleManager.PreBlock(ctx)
}

// BeginBlocker runs module begin-blockers.
func (app *App) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	return app.ModuleManager.BeginBlock(ctx)
}

// Configurator returns the module configurator.
func (app *App) Configurator() module.Configurator { return app.configurator }

// InitChainer initializes the chain from genesis.
func (app *App) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState GenesisState
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	if err := app.UpgradeKeeper.SetModuleVersionMap(ctx, app.ModuleManager.GetVersionMap()); err != nil {
		return nil, err
	}
	return app.ModuleManager.InitGenesis(ctx, app.appCodec, genesisState)
}

// LoadHeight loads state at a height.
func (app *App) LoadHeight(height int64) error { return app.LoadVersion(height) }

// LegacyAmino returns the amino codec.
func (app *App) LegacyAmino() *codec.LegacyAmino { return app.legacyAmino }

// AppCodec returns the app codec.
func (app *App) AppCodec() codec.Codec { return app.appCodec }

// InterfaceRegistry returns the interface registry.
func (app *App) InterfaceRegistry() types.InterfaceRegistry { return app.interfaceRegistry }

// TxConfig returns the tx config.
func (app *App) TxConfig() client.TxConfig { return app.txConfig }

// SimulationManager is unused; simulations are out of scope.
func (app *App) SimulationManager() *module.SimulationManager { return nil }

// AutoCliOpts returns AutoCLI options for every module.
func (app *App) AutoCliOpts() autocli.AppOptions {
	modules := make(map[string]appmodule.AppModule)
	for _, m := range app.ModuleManager.Modules {
		if moduleWithName, ok := m.(module.HasName); ok {
			if appModule, ok := moduleWithName.(appmodule.AppModule); ok {
				modules[moduleWithName.Name()] = appModule
			}
		}
	}
	return autocli.AppOptions{
		Modules:               modules,
		ModuleOptions:         runtimeservices.ExtractAutoCLIOptions(app.ModuleManager.Modules),
		AddressCodec:          authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		ValidatorAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		ConsensusAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	}
}

// DefaultDenomMetadata is the bank denom metadata for the native token (SPEC §4.1).
func DefaultDenomMetadata() banktypes.Metadata {
	return banktypes.Metadata{
		Description: "The native token of Glass Harbor Protocol",
		Base:        BondDenom,
		Display:     DisplayDenom,
		Name:        "Glass",
		Symbol:      DisplayDenom,
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: BondDenom, Exponent: 0},
			{Denom: DisplayDenom, Exponent: 6},
		},
	}
}

// chainGenesisDefaults supplies chain defaults to both App.DefaultGenesis and harbord init.
type chainGenesisDefaults struct {
	module.AppModuleBasic
}

var (
	_ module.AppModuleBasic   = chainGenesisDefaults{}
	_ module.HasGenesisBasics = chainGenesisDefaults{}
)

func (b chainGenesisDefaults) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	bz := b.inner().DefaultGenesis(cdc)
	switch b.Name() {
	case banktypes.ModuleName:
		var gs banktypes.GenesisState
		cdc.MustUnmarshalJSON(bz, &gs)
		gs.DenomMetadata = append(gs.DenomMetadata, DefaultDenomMetadata())
		return cdc.MustMarshalJSON(&gs)
	case govtypes.ModuleName:
		var gs govv1.GenesisState
		cdc.MustUnmarshalJSON(bz, &gs)
		period := 7 * 24 * time.Hour
		gs.Params.VotingPeriod = &period
		return cdc.MustMarshalJSON(&gs)
	default:
		return bz
	}
}

func (b chainGenesisDefaults) ValidateGenesis(cdc codec.JSONCodec, txCfg client.TxEncodingConfig, bz json.RawMessage) error {
	return b.inner().ValidateGenesis(cdc, txCfg, bz)
}

func (b chainGenesisDefaults) inner() module.HasGenesisBasics {
	inner, ok := b.AppModuleBasic.(module.HasGenesisBasics)
	if !ok {
		panic("module basics do not implement module.HasGenesisBasics")
	}
	return inner
}

// DefaultGenesis returns the default genesis of every module.
func (app *App) DefaultGenesis() map[string]json.RawMessage {
	return app.BasicModuleManager.DefaultGenesis(app.appCodec)
}

// GetKey returns a store key (tests).
func (app *App) GetKey(storeKey string) *storetypes.KVStoreKey { return app.keys[storeKey] }

// RegisterAPIRoutes registers REST routes.
func (app *App) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	cmtservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	nodeservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	app.BasicModuleManager.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	if apiConfig.Swagger {
		spec := func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write(docs.OpenAPI)
		}
		// Registered before RegisterSwaggerAPI so it shadows the SDK's stock spec inside the bundled UI (mux matches in order).
		apiSvr.Router.HandleFunc("/swagger/swagger.yaml", spec)
		apiSvr.Router.HandleFunc("/openapi.yaml", spec)
	}
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
}

// RegisterTxService registers the tx gRPC service.
func (app *App) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.GRPCQueryRouter(), clientCtx, app.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService registers CometBFT gRPC queries.
func (app *App) RegisterTendermintService(clientCtx client.Context) {
	cmtApp := server.NewCometABCIWrapper(app)
	cmtservice.RegisterTendermintService(clientCtx, app.GRPCQueryRouter(), app.interfaceRegistry, cmtApp.Query)
}

// RegisterNodeService registers the node gRPC service.
func (app *App) RegisterNodeService(clientCtx client.Context, cfg config.Config) {
	nodeservice.RegisterNodeService(clientCtx, app.GRPCQueryRouter(), cfg, func() int64 {
		return app.CommitMultiStore().EarliestVersion()
	})
}

// GetMaccPerms returns a copy of the module account permissions.
func GetMaccPerms() map[string][]string { return maps.Clone(maccPerms) }

// BlockedAddresses returns module accounts that may not receive funds (gov excepted).
func BlockedAddresses() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range GetMaccPerms() {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}
	delete(modAccAddrs, authtypes.NewModuleAddress(govtypes.ModuleName).String())
	return modAccAddrs
}
