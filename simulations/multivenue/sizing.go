package multivenue

// venueSizedQty bounds an intended quantity by the size visible at the touch
// and reports whether what remains is worth sending.
//
// Clamping to the visible size is what makes a marketable order bounded, but it
// can land below the venue's minimum order size, and the venue rejects those
// outright. A desk that resubmits on the next tick then spends its whole
// request budget on orders that cannot be accepted: measured in an 8h reference
// run, 41094 such rejects for the carry desk and 8302 for the metaorder desk.
//
// A minimum of zero disables the check, which is correct for a venue that has
// no minimum.
func venueSizedQty(intended, available, minOrderSize int64) (int64, bool) {
	if intended <= 0 {
		return 0, false
	}
	if available > 0 && intended > available {
		intended = available
	}
	if intended <= 0 {
		return 0, false
	}
	if minOrderSize > 0 && intended < minOrderSize {
		return 0, false
	}
	return intended, true
}
