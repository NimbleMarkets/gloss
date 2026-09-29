package document

import (
	"fmt"
	"math"
)

type VisionProfile struct{ MaxEdge, PatchSize, PatchBudget int }

// Sizing budgets, not API settings or exact billed-token estimates. Verified
// against provider documentation on 2026-09-29. Callers must still choose the
// corresponding model/detail mode when submitting the resulting PNG.
// https://developers.openai.com/api/docs/guides/images-vision
// https://platform.claude.com/docs/en/build-with-claude/vision
var VisionProfiles = map[string]VisionProfile{
	// Conservative common high-detail envelope for GPT-6 Astra and GPT-5.6.
	"openai-high":     {MaxEdge: 2048, PatchSize: 32, PatchBudget: 2500},
	"claude-standard": {MaxEdge: 1568, PatchSize: 28, PatchBudget: 1568},
	"claude-high":     {MaxEdge: 2576, PatchSize: 28, PatchBudget: 4784},
}

// VisionSize finds the largest uniformly scaled image fitting the edge and
// rounded-up patch limits. It never enlarges a raster or distorts its aspect.
func VisionSize(width, height, maxEdge int, name string) (int, int, error) {
	if width < 1 || height < 1 || maxEdge < 1 || maxEdge > 4096 {
		return 0, 0, fmt.Errorf("invalid image dimensions")
	}
	profile := VisionProfile{MaxEdge: maxEdge}
	if name != "" {
		var ok bool
		profile, ok = VisionProfiles[name]
		if !ok {
			return 0, 0, fmt.Errorf("unknown vision profile %q", name)
		}
		maxEdge = min(maxEdge, profile.MaxEdge)
	}
	upper := math.Min(1, float64(maxEdge)/float64(max(width, height)))
	dims := func(scale float64) (int, int) {
		return max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))
	}
	fits := func(w, h int) bool {
		if profile.PatchSize == 0 {
			return true
		}
		p := profile.PatchSize
		return ((w+p-1)/p)*((h+p-1)/p) <= profile.PatchBudget
	}
	w, h := dims(upper)
	if fits(w, h) {
		return w, h, nil
	}
	lower := float64(0)
	for range 60 {
		mid := (lower + upper) / 2
		w, h := dims(mid)
		if fits(w, h) {
			lower = mid
		} else {
			upper = mid
		}
	}
	w, h = dims(lower)
	return w, h, nil
}
