(.provenance | type) == "object" and
(.provenance.treatment | type) == "object" and
(.provenance.control | type) == "object" and
.provenance.treatment.seed == $expected_seed and
.provenance.control.seed == $expected_seed and
.provenance.treatment.seed == .provenance.control.seed and
.valid == true and
.treatment.supplier_count > 0 and
.control.supplier_count == 0
