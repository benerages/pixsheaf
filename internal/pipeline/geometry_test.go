package pipeline

import "testing"

func ptr(v int) *int { return &v }

func TestConditionMatches(t *testing.T) {
	f := Facts{Size: Size{Width: 4000, Height: 3000}, SourceFormat: "heif"}
	cases := []struct {
		name string
		c    *Condition
		want bool
	}{
		{"nil", nil, true},
		{"widthGt true", &Condition{WidthGt: ptr(3999)}, true},
		{"widthGt false", &Condition{WidthGt: ptr(4000)}, false},
		{"heightLt", &Condition{HeightLt: ptr(3000)}, false},
		{"longestEdgeGt", &Condition{LongestEdgeGt: ptr(2000)}, true},
		{"source match", &Condition{SourceFormat: []string{"jpeg", "heif"}}, true},
		{"source miss", &Condition{SourceFormat: []string{"jpeg"}}, false},
		{"landscape", &Condition{Orientation: "landscape"}, true},
		{"portrait", &Condition{Orientation: "portrait"}, false},
		{"all must hold", &Condition{WidthGt: ptr(100), Orientation: "square"}, false},
	}
	for _, tc := range cases {
		if got := tc.c.Matches(f); got != tc.want {
			t.Errorf("%s: Matches = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestResizePlan(t *testing.T) {
	cur := Size{Width: 4000, Height: 3000}
	cases := []struct {
		name string
		r    Resize
		want Size
		crop bool
	}{
		{"width", Resize{Width: 1000}, Size{1000, 750}, false},
		{"height", Resize{Height: 600}, Size{800, 600}, false},
		{"longest edge", Resize{LongestEdge: 1600}, Size{1600, 1200}, false},
		{"percent", Resize{Percent: 25}, Size{1000, 750}, false},
		{"fit", Resize{Width: 1000, Height: 1000, Mode: "fit"}, Size{1000, 750}, false},
		{"fill", Resize{Width: 1000, Height: 1000, Mode: "fill"}, Size{1000, 1000}, false},
		{"crop", Resize{Width: 1000, Height: 1000, Mode: "crop", Gravity: "center"}, Size{1000, 1000}, true},
		{"no enlarge", Resize{Width: 8000}, Size{4000, 3000}, false},
		{"enlarge", Resize{Width: 8000, Enlarge: true}, Size{8000, 6000}, false},
		{"crop no enlarge", Resize{Width: 5000, Height: 1000, Mode: "crop", Gravity: "center"}, Size{4000, 1000}, true},
	}
	for _, tc := range cases {
		p := tc.r.Plan(cur)
		if got := p.Result(cur); got != tc.want {
			t.Errorf("%s: result %v, want %v", tc.name, got, tc.want)
		}
		if (p.Crop != nil) != tc.crop {
			t.Errorf("%s: crop = %v, want %v", tc.name, p.Crop, tc.crop)
		}
	}
}

func TestResizeCropGravity(t *testing.T) {
	p := Resize{Width: 100, Height: 100, Mode: "crop", Gravity: "west"}.Plan(Size{400, 200})
	// Scaled to 200x100; the box keeps the left 100 pixels.
	if p.Crop == nil || *p.Crop != (Box{Left: 0, Top: 0, Width: 100, Height: 100}) || p.Smart != "" {
		t.Errorf("crop = %+v, smart %q", p.Crop, p.Smart)
	}
	p = Resize{Width: 100, Height: 100, Mode: "crop", Gravity: "attention"}.Plan(Size{400, 200})
	if p.Smart != "attention" {
		t.Errorf("smart = %q, want attention", p.Smart)
	}
}

func TestCropBox(t *testing.T) {
	cur := Size{Width: 400, Height: 300}
	cases := []struct {
		name string
		c    Crop
		want Box
		ok   bool
	}{
		{"square center", Crop{AspectRatio: &AspectRatio{1, 1}, Gravity: "center"}, Box{50, 0, 300, 300}, true},
		{"square east", Crop{AspectRatio: &AspectRatio{1, 1}, Gravity: "east"}, Box{100, 0, 300, 300}, true},
		{"wide south", Crop{AspectRatio: &AspectRatio{2, 1}, Gravity: "south"}, Box{0, 100, 400, 200}, true},
		{"rect", Crop{Rect: &Rect{10, 20, 100, 50}}, Box{10, 20, 100, 50}, true},
		{"rect clipped", Crop{Rect: &Rect{350, 250, 100, 100}}, Box{350, 250, 50, 50}, true},
		{"rect outside", Crop{Rect: &Rect{400, 0, 10, 10}}, Box{}, false},
	}
	for _, tc := range cases {
		got, _, ok := tc.c.Box(cur)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: Box = %+v, %v; want %+v, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPlaceWithMargin(t *testing.T) {
	got := Place(Size{1000, 800}, Size{200, 100}, "southeast", 10)
	if got != (Box{Left: 790, Top: 690, Width: 200, Height: 100}) {
		t.Errorf("Place = %+v", got)
	}
}

func TestDefaultOutputFormat(t *testing.T) {
	if DefaultOutputFormat("heif") != "jpeg" || DefaultOutputFormat("png") != "png" {
		t.Error("DefaultOutputFormat")
	}
}
