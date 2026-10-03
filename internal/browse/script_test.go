package browse_test

import (
	"testing"

	"github.com/NimbleMarkets/gloss/internal/browse/browsecfg"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

func TestScripts(t *testing.T) {
	browsetest.RunDir(t, browsecfg.Config(), "testdata/scripts")
}
