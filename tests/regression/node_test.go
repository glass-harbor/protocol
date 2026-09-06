//go:build regression

package regression

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/stretchr/testify/require"
)

const chainID = "glassharbor-regtest-1"

// Fixed mnemonics so addresses are stable across runs and usable in YAML via {{ addr NAME }}.
var keyMnemonics = []struct{ name, mnemonic string }{
	{"validator", "gaze bag search west promote avocado fly shield book category mention peanut rose sound jeans cave opera sun axis mansion bomb process toe sport"},
	{"treasury", "local prepare grunt observe race shy unique metal tiger rocket nest gasp guide mask music ridge melody length expire coin resource globe security link"},
	{"alice", "observe opera arrange belt inform debate fury will valve runway casual text pull bar inject rack giggle rhythm pluck ecology lift project vibrant sponsor"},
	{"bob", "perfect shed concert phrase chimney long veteran portion custom maze frog tent weird predict foil immense wrap pyramid brick thrive powder august muffin blush"},
	{"carol", "blur industry cactus episode jungle rocket field panel toddler gate pulp allow exercise index rice mountain ozone erode agent hurt submit duty burden cost"},
}

var (
	regtestBin string // resolved in TestMain
	baseHome   string // prepared once in TestMain, copied per suite
)

// pendingTx is a tx accepted by CheckTx whose block result has not been verified yet.
type pendingTx struct {
	hash   string
	errSub string
	o      op
}

// signer tracks account number and next sequence for one key.
type signer struct{ accNum, seq uint64 }

type node struct {
	t       *testing.T
	path    string // suite file, for messages
	home    string
	gate    string // http://127.0.0.1:PORT
	api     string // http://127.0.0.1:PORT
	rpcURL  string
	rpc     *rpchttp.HTTP
	pending []pendingTx
	signers map[string]*signer
}

// run executes a harbord CLI command against the base home and fails the test run on error.
func run(stdin string, args ...string) string {
	cmd := exec.Command(regtestBin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n%s\n", regtestBin, strings.Join(args, " "), out)
		panic(err)
	}
	return string(out)
}

// prepareBaseHome mirrors scripts/localnet.sh init with fixed keys and regtest consensus settings.
func prepareBaseHome(home string) {
	kr := []string{"--keyring-backend", "test", "--home", home}
	run("", "init", "regtest", "--chain-id", chainID, "--default-denom", "uglass", "--home", home)
	for _, k := range keyMnemonics {
		run(k.mnemonic+"\n", append([]string{"keys", "add", k.name, "--recover"}, kr...)...)
		addr := strings.TrimSpace(run("", append([]string{"keys", "show", k.name, "-a"}, kr...)...))
		run("", "genesis", "add-genesis-account", addr, "1000000000000uglass", "--home", home)
	}
	run("", append([]string{"genesis", "gentx", "validator", "100000000000uglass", "--chain-id", chainID}, kr...)...)
	run("", "genesis", "collect-gentxs", "--home", home)
	run("", "genesis", "validate", "--home", home)
	// CometBFT settings without start flags: propose the next block immediately after commit,
	// and never let the proposer timeout fire while the block is parked in the gate.
	setToml(filepath.Join(home, "config", "config.toml"), "skip_timeout_commit", "true")
	setToml(filepath.Join(home, "config", "config.toml"), "timeout_propose", `"1h"`)
}

// setToml replaces the value of a top-level `key = ...` line.
func setToml(path, key, value string) {
	bz, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + ` = .*$`)
	if !re.Match(bz) {
		panic(fmt.Sprintf("%s: key %q not found", path, key))
	}
	if err := os.WriteFile(path, re.ReplaceAll(bz, []byte(key+" = "+value)), 0o600); err != nil {
		panic(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startNode copies the base home, merges genesis overlays, starts harbord-regtest and waits for it.
func startNode(t *testing.T, path string, overlays [][]byte) *node {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	out, err := exec.Command("cp", "-R", baseHome, home).CombinedOutput()
	require.NoError(t, err, string(out))
	genesis := filepath.Join(home, "config", "genesis.json")
	for _, overlay := range overlays {
		mergeGenesis(t, genesis, overlay)
	}

	rpcPort, p2pPort, grpcPort, apiPort, gatePort := freePort(t), freePort(t), freePort(t), freePort(t), freePort(t)
	n := &node{
		t: t, path: path, home: home,
		gate:    fmt.Sprintf("http://127.0.0.1:%d", gatePort),
		api:     fmt.Sprintf("http://127.0.0.1:%d", apiPort),
		rpcURL:  fmt.Sprintf("http://127.0.0.1:%d", rpcPort),
		signers: map[string]*signer{},
	}
	n.rpc, err = rpchttp.New(n.rpcURL, "/websocket")
	require.NoError(t, err)

	logPath := filepath.Join(t.TempDir(), "node.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	var w io.Writer = logFile
	if os.Getenv("DEBUG") != "" {
		w = io.MultiWriter(logFile, os.Stderr)
	}
	cmd := exec.Command(regtestBin, "start", "--home", home,
		"--rpc.laddr", fmt.Sprintf("tcp://127.0.0.1:%d", rpcPort),
		"--p2p.laddr", fmt.Sprintf("tcp://127.0.0.1:%d", p2pPort),
		"--grpc.address", fmt.Sprintf("127.0.0.1:%d", grpcPort),
		"--api.address", fmt.Sprintf("tcp://127.0.0.1:%d", apiPort),
		"--api.enable=true",
		"--rpc.pprof_laddr=",
		"--mempool.max-txs", "5000",
		"--minimum-gas-prices", "0uglass",
	)
	cmd.Env = append(os.Environ(), "HARBORD_REGTEST_ADDR="+fmt.Sprintf("127.0.0.1:%d", gatePort))
	cmd.Stdout, cmd.Stderr = w, w
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
		if t.Failed() {
			bz, _ := os.ReadFile(logPath)
			lines := strings.Split(strings.TrimSpace(string(bz)), "\n")
			if len(lines) > 100 {
				lines = lines[len(lines)-100:]
			}
			t.Logf("node log tail (%s):\n%s", logPath, strings.Join(lines, "\n"))
		}
	})

	// Wait for the gate, then produce the genesis block ourselves: the SDK refuses queries
	// ("is not ready; please wait for first block") until block 1 is committed. Every suite
	// therefore starts at height 1 and its first create-blocks yields height 2.
	deadline := time.Now().Add(60 * time.Second)
	for {
		if code, _ := n.httpGet(n.gate + "/ping"); code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: regtest gate did not answer /ping in 60s", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if h := n.newBlock(); h != 1 {
		t.Fatalf("%s: first /newBlock returned height %d, want 1", path, h)
	}
	for {
		if code, _ := n.httpGet(n.api + "/cosmos/auth/v1beta1/params"); code == http.StatusOK {
			return n
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: REST API did not become ready in 60s", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// mergeGenesis deep-merges a JSON overlay into genesis.json with jq.
func mergeGenesis(t *testing.T, genesis string, overlay []byte) {
	t.Helper()
	overlayPath := genesis + ".overlay.json"
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	cmd := exec.Command("jq", "-s", ".[0] * .[1]", genesis, overlayPath)
	out, err := cmd.Output()
	require.NoError(t, err, "jq merge failed")
	require.NoError(t, os.WriteFile(genesis, out, 0o600))
}

// httpGet returns status and body; a transport error is status 0.
func (n *node) httpGet(url string) (int, []byte) {
	res, err := http.Get(url) // gosec G107 (variable URL) is already excluded in .golangci.yml
	if err != nil {
		return 0, nil
	}
	defer res.Body.Close()
	bz, _ := io.ReadAll(res.Body)
	return res.StatusCode, bz
}

// newBlock releases one block and returns the committed height.
func (n *node) newBlock() int64 {
	code, body := n.httpGet(n.gate + "/newBlock")
	if code != http.StatusOK {
		n.t.Fatalf("%s: /newBlock returned %d: %s (node may have died; see log tail)", n.path, code, body)
	}
	h, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
	require.NoError(n.t, err)
	return h
}

func (n *node) fatalf(o op, format string, args ...any) {
	n.t.Helper()
	n.t.Fatalf("%s:%d (%s): %s", n.path, o.line, o.Type, fmt.Sprintf(format, args...))
}

func (n *node) ctx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	n.t.Cleanup(cancel)
	return ctx
}
