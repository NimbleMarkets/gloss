package document

import "testing"

func TestVisionProfilesAreEdges(t *testing.T) {
	for name, p := range VisionProfiles {
		if p.MaxEdge < 1 || p.MaxEdge > 4096 || p.Reason == "" {
			t.Fatalf("%s: %+v", name, p)
		}
		w, h, err := VisionSize(4096, 4096, p.MaxEdge)
		if err != nil || w != p.MaxEdge || h != p.MaxEdge {
			t.Fatalf("%s: %dx%d %v", name, w, h, err)
		}
	}
}

func TestVisionSizeKeepsAspectAndNeverEnlarges(t *testing.T) {
	for _, size := range [][2]int{{3840, 2160}, {612, 792}, {3000, 500}, {64, 64}, {1, 10000}} {
		w, h, err := VisionSize(size[0], size[1], 1000)
		if err != nil || w > size[0] || h > size[1] || max(w, h) > 1000 {
			t.Fatalf("%v: %dx%d %v", size, w, h, err)
		}
	}
}

func TestResolveEdge(t *testing.T) {
	if edge, reason, err := ResolveEdge(1536, false, ""); err != nil || edge != 1536 || reason != "" {
		t.Fatal(edge, reason, err)
	}
	if edge, reason, err := ResolveEdge(1536, false, "claude-standard"); err != nil || edge != 1092 || reason == "" {
		t.Fatal(edge, reason, err)
	}
	if edge, reason, err := ResolveEdge(512, true, "claude-high"); err != nil || edge != 512 || reason == "" {
		t.Fatal(edge, reason, err)
	}
	if _, _, err := ResolveEdge(1536, false, "gpt-9"); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
