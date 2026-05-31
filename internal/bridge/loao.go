package bridge

// Leave-one-anchor-out (LOAO) validation: the bridge's empirical credibility
// test. Drop each interior anchor in turn, re-fit the bridge on the rest, predict
// the held-out anchor's position, and check that anchor's own identified
// posterior falls within the bridge's predicted interval. LOAO coverage — the
// fraction of held-out anchors covered — is the bridge analogue of the identified
// marks' calibration study and ships as a headline dossier number.

// LOAORow is one leave-one-anchor-out trial.
type LOAORow struct {
	HeldMarkID    string
	PolicyPoint   float64
	AnchorCentral float64
	PredLower     float64
	PredUpper     float64
	Covered       bool
	Skipped       bool // endpoint anchors cannot be held out without breaking bracketing
	SkipReason    string
}

// LOAOReport aggregates the per-anchor trials and the headline coverage.
type LOAOReport struct {
	Level    float64
	Rows     []LOAORow
	Coverage float64 // fraction of NON-skipped held-out anchors that were covered
}

// LeaveOneAnchorOut runs the LOAO battery. An endpoint anchor (the global min or
// max on the policy variable) cannot be held out, because the remaining anchors
// would no longer bracket it — that is extrapolation, which is out of scope; such
// anchors are reported as skipped rather than failed.
func LeaveOneAnchorOut(mech Mechanism, anchors []Anchor, k Kernel, level float64, cfg SMCConfig) LOAOReport {
	rep := LOAOReport{Level: level}
	minX, maxX := anchors[0].X, anchors[0].X
	for _, a := range anchors {
		if a.X < minX {
			minX = a.X
		}
		if a.X > maxX {
			maxX = a.X
		}
	}

	var covered, scored int
	for i, held := range anchors {
		row := LOAORow{HeldMarkID: held.MarkID, PolicyPoint: held.X, AnchorCentral: held.mean()}
		if held.X <= minX || held.X >= maxX {
			row.Skipped = true
			row.SkipReason = "endpoint anchor — holding it out would require extrapolation"
			rep.Rows = append(rep.Rows, row)
			continue
		}
		rest := make([]Anchor, 0, len(anchors)-1)
		for j, a := range anchors {
			if j != i {
				rest = append(rest, a)
			}
		}
		post, err := Calibrate(mech, rest, held.X, k, cfg)
		if err != nil {
			row.Skipped = true
			row.SkipReason = "calibration failed: " + err.Error()
			rep.Rows = append(rep.Rows, row)
			continue
		}
		lo, hi := post.Interval(level)
		row.PredLower, row.PredUpper = lo, hi
		row.Covered = held.mean() >= lo && held.mean() <= hi
		if row.Covered {
			covered++
		}
		scored++
		rep.Rows = append(rep.Rows, row)
	}
	if scored > 0 {
		rep.Coverage = float64(covered) / float64(scored)
	}
	return rep
}
