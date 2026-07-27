package router
 
// affinityMatrix holds base scores representing how well each engine
// historically performs for each intent class.
// Seeded with honest tier-based values grounded in engine research.
// H=0.80, M=0.60, L=0.30 — broad tiers, not fake precision.
// Phase 3 activates EMA updates from real query outcomes.
var affinityMatrix = map[string]map[string]float64{
	"news": {
		"brave":  0.80,
		"ddg":    0.60,
		"bing":   0.80,
		"mojeek": 0.30,
		"yandex": 0.60,
	},
	"code": {
		"brave":  0.80,
		"ddg":    0.80,
		"bing":   0.80,
		"mojeek": 0.30,
		"yandex": 0.20,
	},
	"factual": {
		"brave":  0.60,
		"ddg":    0.80,
		"bing":   0.60,
		"mojeek": 0.60,
		"yandex": 0.30,
	},
	"research": {
		"brave":  0.80,
		"ddg":    0.60,
		"bing":   0.60,
		"mojeek": 0.60,
		"yandex": 0.30,
	},
	"commercial": {
		"brave":  0.60,
		"ddg":    0.60,
		"bing":   0.80,
		"mojeek": 0.30,
		"yandex": 0.30,
	},
	"general": {
		"brave":  0.80,
		"ddg":    0.80,
		"bing":   0.60,
		"mojeek": 0.60,
		"yandex": 0.60,
	},
}

// indexOverlap lists engine pairs that share the same underlying index.
// Selecting both from a pair returns duplicate results.
// The scorer penalises the lower-scoring engine in each pair.
var indexOverlap = [][2]string{
	{"ddg", "bing"},
}

// anchors are high-reliability engines that must always appear in selection.
// If scoring would exclude all anchors, the highest-scoring available anchor
// is force-included.
var anchors = []string{"brave", "ddg"}

// affinityFor returns the base affinity score for an engine given an intent.
// Returns 0.30 for any engine or intent not explicitly in the matrix —
// unknown engines get a low but non-zero score so they can still be selected
// if all known engines are unhealthy.
func affinityFor(intent, engine string) float64 {
	row, ok := affinityMatrix[intent]
	if !ok {
		return 0.30
	}
	score, ok := row[engine]
	if !ok {
		return 0.30
	}
	return score
}

// updateAffinity nudges an engine's affinity score for an intent toward the
// outcome signal using an exponential moving average.
// Called by RecordOutcome in Phase 3. Stubbed and unused in Phase 1.
func updateAffinity(intent, engine string, outcomeSignal float64) {
	row, ok := affinityMatrix[intent]
	if !ok {
		return
	}
	current, ok := row[engine]
	if !ok {
		current = 0.30
	}
	row[engine] = current*0.95 + outcomeSignal*0.05
}