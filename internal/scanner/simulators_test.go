package scanner

import "testing"

func TestUnavailableSimulatorsRemainInspectionOnly(t *testing.T) {
	data := []byte(`{"devices":{"runtime":[{"name":"Old phone","udid":"11111111-2222-3333-4444-555555555555","isAvailable":false},{"name":"Current","udid":"22222222-2222-3333-4444-555555555555","isAvailable":true},{"name":"Invalid","udid":"../../private","isAvailable":false}]}}`)
	items, err := simulatorCandidates(data, "/fixture/Devices")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "Old phone" || items[0].Selectable() {
		t.Fatalf("unexpected simulator targets: %+v", items)
	}
}
