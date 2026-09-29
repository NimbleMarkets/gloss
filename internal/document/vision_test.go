package document

import "testing"

func TestVisionProfiles(t *testing.T) {
	for name, p := range VisionProfiles {
		for _, size := range [][2]int{{4096, 4096}, {3840, 2160}, {612, 792}, {3000, 500}, {64, 64}, {1, 10000}} {
			w, h, err := VisionSize(size[0], size[1], 4096, name)
			if err != nil {
				t.Fatal(err)
			}
			if w > size[0] || h > size[1] || max(w, h) > p.MaxEdge {
				t.Fatalf("%s invalid dimensions %dx%d", name, w, h)
			}
			if ((w+p.PatchSize-1)/p.PatchSize)*((h+p.PatchSize-1)/p.PatchSize) > p.PatchBudget {
				t.Fatalf("%s exceeds patch budget", name)
			}
		}
	}
	for name, want := range map[string]int{"openai-high": 1600, "claude-standard": 1092, "claude-high": 1932} {
		w, h, err := VisionSize(4096, 4096, 4096, name)
		if err != nil || w != want || h != want {
			t.Fatalf("%s: %dx%d %v", name, w, h, err)
		}
	}
}
