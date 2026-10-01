package document

import (
	"fmt"
	"math"
	"slices"
)

// VisionProfile is a convenience alias for a maximum edge. Vendors change
// their image budgets, so the aliases go stale: a caller that knows its
// model's budget should pass --max-edge, which is the stable interface.
type VisionProfile struct {
	MaxEdge int
	Reason  string // Why this edge, in a line, for the manifest.
}

// VisionProfiles are the aliases, each the largest edge at which an image of
// any shape stays within the provider's patch budget when it is square, the
// worst case. Sizing budgets, not API settings or billed-token estimates;
// checked against provider documentation on 2026-09-29. Callers must still
// choose the corresponding model and detail mode when submitting the PNG.
// https://developers.openai.com/api/docs/guides/images-vision
// https://platform.claude.com/docs/en/build-with-claude/vision
//
// No further names are added: new models are for --max-edge.
var VisionProfiles = map[string]VisionProfile{
	"openai-high":     {MaxEdge: 1600, Reason: "square-safe edge for a 2500-patch budget of 32 px patches, 2048 px at most"},
	"claude-standard": {MaxEdge: 1092, Reason: "square-safe edge for a 1568-patch budget of 28 px patches"},
	"claude-high":     {MaxEdge: 1932, Reason: "square-safe edge for a 4784-patch budget of 28 px patches, 2576 px at most"},
}

// VisionProfileNames lists the aliases, sorted.
func VisionProfileNames() []string {
	names := make([]string, 0, len(VisionProfiles))
	for name := range VisionProfiles {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ResolveEdge settles the edge an export is sized to. An edge given
// outright is the caller's and wins; otherwise a profile names one. It
// returns the edge and, when a profile was named, its reason.
func ResolveEdge(edge int, edgeGiven bool, profile string) (int, string, error) {
	if profile == "" {
		return edge, "", nil
	}
	p, ok := VisionProfiles[profile]
	if !ok {
		return 0, "", fmt.Errorf("unknown vision profile %q", profile)
	}
	if edgeGiven {
		return edge, "--max-edge given, so the alias is not applied", nil
	}
	return p.MaxEdge, p.Reason, nil
}

// VisionSize finds the largest uniformly scaled size fitting the edge. It
// never enlarges a raster or distorts its aspect.
func VisionSize(width, height, maxEdge int) (int, int, error) {
	if width < 1 || height < 1 || maxEdge < 1 || maxEdge > 4096 {
		return 0, 0, fmt.Errorf("invalid image dimensions")
	}
	scale := math.Min(1, float64(maxEdge)/float64(max(width, height)))
	return max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale)), nil
}
