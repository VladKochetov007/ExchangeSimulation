package analysis

import "testing"

func TestPositionReconstructionRetainsZeroMarkAsPresent(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"OIL-FUT","new_size":1,"new_entry_price":20}}}`,
			`{"sim_ts":1,"client_id":2,"event":"position_update","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":2,"symbol":"OIL-FUT","new_size":-1,"new_entry_price":20}}}`,
			`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","payload":{"timestamp":2,"symbol":"OIL-FUT","mark_price":0}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	reconstruction, err := run.MeasurePositions(PositionOptions{BasePrecision: 1})
	if err != nil {
		t.Fatalf("measure positions: %v", err)
	}
	if len(reconstruction.Contracts) != 1 {
		t.Fatalf("contracts = %#v, want one OIL-FUT contract", reconstruction.Contracts)
	}
	contract := reconstruction.Contracts[0]
	if !contract.MarkAvailable || contract.MarkPrice != 0 {
		t.Fatalf("zero mark contract = %#v, want MarkAvailable at zero", contract)
	}
	if contract.OpenValue != 0 || reconstruction.UnrepresentableOpenValues != 0 {
		t.Fatalf("zero-mark reconstructed PnL = open=%d unrepresentable=%d, want 0/0", contract.OpenValue, reconstruction.UnrepresentableOpenValues)
	}
}
