package forecast

import "testing"

func TestExpects(t *testing.T) {
	f := Forecast{Breaking: []Change{{Symbol: "jwt.StandardClaims", Description: "replaced by RegisteredClaims"}}}
	if !f.Expects("jwt.StandardClaims") {
		t.Error("Expects missed a forecast symbol")
	}
	if f.Expects("jwt.Parse") {
		t.Error("Expects matched a symbol that was not forecast")
	}
}
