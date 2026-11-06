package results

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestGoJSONReadsLongEventsAndRejectsMalformedEvents(t *testing.T) {
	long, err := json.Marshal(map[string]string{"Action": "output", "Output": strings.Repeat("x", 128*1024)})
	if err != nil {
		t.Fatal(err)
	}
	input := string(long) + "\n" + `{"Action":"fail","Package":"example","Test":"TestFail"}` + "\n"
	got, err := ParseGoJSON(input)
	if err != nil || len(got) != 1 || got[0].Status != domain.StatusFailed {
		t.Fatalf("results: %+v, error: %v", got, err)
	}
	if _, err := ParseGoJSON(input + "{broken\n"); err == nil {
		t.Fatal("malformed event was silently ignored")
	}
}
