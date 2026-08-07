package reclaim

// CheckOccupancyForTest exposes the orphan scan's I12 guard to the package's
// external test. The guard sits between two indexer calls that a test cannot
// make, so it is reached directly rather than not at all.
var CheckOccupancyForTest = checkOccupancy

// PlanSweepForTest exposes the sweep's decision so shared-slab correctness can
// be checked without spending storage on every case.
var PlanSweepForTest = planSweep
