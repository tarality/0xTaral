// this is a dummy package only for checking builds
package main

import (
	"fmt"
	"io"

	"github.com/tarality/0xTaral/core"
)

func main() {
	b := core.NewIBFT(nil, nil, nil)

	// prevent golang compiler from removing the whole function
	_, _ = fmt.Fprint(io.Discard, b)
}
