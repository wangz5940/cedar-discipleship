package studymemory

import (
	"math"
	"testing"
)

func TestHandoutCueValidation(t *testing.T) {
	if !ValidHandout(Handout{AssetID: 10, Pages: []PageCue{{Page: 1, Time: 0}, {Page: 2, Time: 100}}}) {
		t.Fatal("valid rejected")
	}
	for _, h := range []Handout{{AssetID: 0}, {AssetID: 10, Pages: []PageCue{{Page: 0}}}, {AssetID: 10, Pages: []PageCue{{Page: 1, Time: math.NaN()}}}, {AssetID: 10, Pages: []PageCue{{Page: 1}, {Page: 1, Time: 3}}}, {AssetID: 10, Pages: []PageCue{{Page: 1, Time: -1}}}} {
		if ValidHandout(h) {
			t.Fatal("invalid accepted")
		}
	}
}
