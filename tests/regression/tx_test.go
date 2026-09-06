//go:build regression

package regression

func (n *node) tx(o op)                     { n.fatalf(o, "tx op not implemented yet") }
func (n *node) verifyPending(o op, h int64) {}
