//go:build regression

package regression

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/app"
)

var (
	txConfig client.TxConfig
	cdc      codec.Codec
	kr       keyring.Keyring
	addrs    = map[string]sdk.AccAddress{}
)

func TestMain(m *testing.M) {
	if _, err := os.Stat("suites"); err != nil {
		fmt.Fprintln(os.Stderr, "run from tests/regression (go test sets the cwd to the package dir)")
		os.Exit(1)
	}
	if _, err := os.Stat(os.Getenv("HARBORD_REGTEST_BIN")); err == nil {
		regtestBin = os.Getenv("HARBORD_REGTEST_BIN")
	} else {
		regtestBin, _ = filepath.Abs("../../build/harbord-regtest")
	}
	if _, err := os.Stat(regtestBin); err != nil {
		fmt.Fprintf(os.Stderr, "%s not found: run `make build-regtest` or set HARBORD_REGTEST_BIN\n", regtestBin)
		os.Exit(1)
	}
	if _, err := exec.LookPath("jq"); err != nil {
		fmt.Fprintln(os.Stderr, "jq is required on PATH")
		os.Exit(1)
	}

	tmp, err := os.MkdirTemp("", "harbord-regression-")
	if err != nil {
		panic(err)
	}

	// Encoding config the same way cmd/harbord/cmd/root.go gets it.
	tempApp := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(filepath.Join(tmp, "encoding")))
	txConfig, cdc = tempApp.TxConfig(), tempApp.AppCodec()

	baseHome = filepath.Join(tmp, "base")
	prepareBaseHome(baseHome)
	kr, err = keyring.New("harbord", keyring.BackendTest, baseHome, nil, cdc)
	if err != nil {
		panic(err)
	}
	for _, k := range keyMnemonics {
		rec, err := kr.Key(k.name)
		if err != nil {
			panic(err)
		}
		addrs[k.name], err = rec.GetAddress()
		if err != nil {
			panic(err)
		}
	}

	code := m.Run() // os.Exit skips deferred calls, so clean up explicitly
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func TestRegression(t *testing.T) {
	var files []string
	err := filepath.WalkDir("suites", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yaml") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no suites found")
	}
	for _, f := range files {
		name := strings.TrimSuffix(strings.TrimPrefix(f, "suites/"), ".yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runSuite(t, f)
		})
	}
}
