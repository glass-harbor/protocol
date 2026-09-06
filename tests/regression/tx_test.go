//go:build regression

package regression

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

const defaultGas = 400_000

// loadSigner fetches account number and sequence over REST on first use (REST goes through the
// SDK gRPC gateway, not CometBFT's ABCI query, so it works while the node is gated).
func (n *node) loadSigner(o op) *signer {
	if s, ok := n.signers[o.Signer]; ok {
		return s
	}
	addr := mustAddr(o.Signer)
	code, body := n.httpGet(n.api + "/cosmos/auth/v1beta1/account_info/" + addr.String())
	if code != http.StatusOK {
		n.fatalf(o, "account_info for %s returned %d: %s", o.Signer, code, body)
	}
	var res struct {
		Info struct {
			AccountNumber string `json:"account_number"`
			Sequence      string `json:"sequence"`
		} `json:"info"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		n.fatalf(o, "account_info decode: %v", err)
	}
	accNum, err := strconv.ParseUint(res.Info.AccountNumber, 10, 64)
	if err != nil {
		n.fatalf(o, "account_number %q: %v", res.Info.AccountNumber, err)
	}
	seq, err := strconv.ParseUint(res.Info.Sequence, 10, 64)
	if err != nil {
		n.fatalf(o, "sequence %q: %v", res.Info.Sequence, err)
	}
	s := &signer{accNum: accNum, seq: seq}
	n.signers[o.Signer] = s
	return s
}

// tx signs the msgs and submits them to the gate.
func (n *node) tx(o op) {
	if o.Signer == "" || len(o.Msgs) == 0 {
		n.fatalf(o, "tx needs signer and msgs")
	}
	msgs := make([]sdk.Msg, 0, len(o.Msgs))
	for i, m := range o.Msgs {
		bz, err := json.Marshal(m)
		if err != nil {
			n.fatalf(o, "msg %d: %v", i, err)
		}
		var msg sdk.Msg
		if err := cdc.UnmarshalInterfaceJSON(bz, &msg); err != nil {
			n.fatalf(o, "msg %d: %v\n%s", i, err, bz)
		}
		msgs = append(msgs, msg)
	}

	s := n.loadSigner(o)
	seq := s.seq
	if o.Sequence != nil {
		seq = *o.Sequence
	}
	gas := o.Gas
	if gas == 0 {
		gas = defaultGas
	}
	txf := clienttx.Factory{}.
		WithTxConfig(txConfig).
		WithKeybase(kr).
		WithChainID(chainID).
		WithAccountNumber(s.accNum).
		WithSequence(seq).
		WithGas(gas).
		WithSignMode(signing.SignMode_SIGN_MODE_DIRECT)
	builder, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		n.fatalf(o, "build tx: %v", err)
	}
	if err := clienttx.Sign(context.Background(), txf, o.Signer, builder, true); err != nil {
		n.fatalf(o, "sign: %v", err)
	}
	bz, err := txConfig.TxEncoder()(builder.GetTx())
	if err != nil {
		n.fatalf(o, "encode: %v", err)
	}

	res, err := http.Post(n.gate+"/tx", "application/octet-stream", bytes.NewReader(bz))
	if err != nil {
		n.fatalf(o, "POST /tx: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		n.fatalf(o, "POST /tx returned %d: %s", res.StatusCode, body)
	}
	var out struct {
		Code uint32 `json:"code"`
		Log  string `json:"log"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		n.fatalf(o, "gate response: %v: %s", err, body)
	}
	if out.Code != 0 {
		if o.Error != "" && strings.Contains(out.Log, o.Error) {
			return // rejected at CheckTx as expected; sequence unchanged
		}
		n.fatalf(o, "tx rejected at CheckTx: code %d: %s", out.Code, out.Log)
	}
	s.seq = seq + 1
	n.pending = append(n.pending, pendingTx{hash: out.Hash, errSub: o.Error, o: o})
}

// verifyPending checks every tx accepted since the last block against block h's results.
func (n *node) verifyPending(o op, h int64) {
	if len(n.pending) == 0 {
		return
	}
	ctx := n.ctx()
	blk, err := n.rpc.Block(ctx, &h)
	if err != nil {
		n.fatalf(o, "rpc block %d: %v", h, err)
	}
	results, err := n.rpc.BlockResults(ctx, &h)
	if err != nil {
		n.fatalf(o, "rpc block_results %d: %v", h, err)
	}
	byHash := map[string]int{}
	for i, tx := range blk.Block.Txs {
		byHash[fmt.Sprintf("%X", tx.Hash())] = i
	}
	for _, p := range n.pending {
		i, ok := byHash[p.hash]
		if !ok {
			n.fatalf(p.o, "tx %s was not included in block %d", p.hash, h)
		}
		r := results.TxsResults[i]
		switch {
		case p.errSub == "" && r.Code != 0:
			n.fatalf(p.o, "tx failed in block %d: code %d: %s", h, r.Code, r.Log)
		case p.errSub != "" && r.Code == 0:
			n.fatalf(p.o, "tx succeeded in block %d but expected error containing %q", h, p.errSub)
		case p.errSub != "" && !strings.Contains(r.Log, p.errSub):
			n.fatalf(p.o, "tx failed in block %d with %q, expected error containing %q", h, r.Log, p.errSub)
		}
	}
	n.pending = nil
}
