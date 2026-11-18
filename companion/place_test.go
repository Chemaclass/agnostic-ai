package main

import "testing"

func TestPlacePanel_OpensBesideTheBubbleOnTheSideWithRoom(t *testing.T) {
	work := rect{X: 0, Y: 25, Width: 1440, Height: 875}
	cases := []struct {
		name         string
		bubble       rect
		wantX, wantY int
		want         corner
	}{
		{"bubble at the lower right", rect{1340, 800, 84, 84}, 1340 - 4 - 400, 800 + 84 - 600, cornerBottomRight},
		{"bubble at the upper left", rect{20, 40, 84, 84}, 20 + 84 + 4, 40, cornerTopLeft},
		{"bubble at the upper right", rect{1340, 40, 84, 84}, 1340 - 4 - 400, 40, cornerTopRight},
		{"bubble at the lower left", rect{20, 800, 84, 84}, 20 + 84 + 4, 800 + 84 - 600, cornerBottomLeft},
		{"bubble pushed past the bottom edge stays on screen", rect{1340, 880, 84, 84}, 1340 - 4 - 400, 25 + 875 - 600, cornerBottomRight},
	}
	for _, tc := range cases {
		x, y, nearest := placePanel(tc.bubble, work, 400, 600)
		if x != tc.wantX || y != tc.wantY || nearest != tc.want {
			t.Errorf("%s: panel at (%d, %d) from %s, want (%d, %d) from %s", tc.name, x, y, nearest, tc.wantX, tc.wantY, tc.want)
		}
	}
}

func TestPlacePanel_StartsAtTheWorkAreaWhenThePanelIsLarger(t *testing.T) {
	x, y, _ := placePanel(rect{100, 100, 84, 84}, rect{X: 10, Y: 20, Width: 300, Height: 400}, 400, 600)
	if x != 10 || y != 20 {
		t.Errorf("panel at (%d, %d), want (10, 20)", x, y)
	}
}

func TestDefaultBubblePosition_IsTheLowerRightOfTheWorkArea(t *testing.T) {
	x, y := defaultBubblePosition(rect{X: 0, Y: 25, Width: 1440, Height: 875}, 84)
	if x != 1332 || y != 792 {
		t.Errorf("bubble at (%d, %d), want (1332, 792)", x, y)
	}
}

func TestOnScreen_SaysWhetherThePersonCanReachTheBubble(t *testing.T) {
	laptop := rect{X: 0, Y: 25, Width: 1440, Height: 875}
	external := rect{X: 1440, Y: 0, Width: 2560, Height: 1440}
	cases := []struct {
		name    string
		bubble  rect
		screens []rect
		want    bool
	}{
		{"on the laptop screen", rect{1332, 792, 84, 84}, []rect{laptop}, true},
		{"on the external monitor", rect{3000, 600, 84, 84}, []rect{laptop, external}, true},
		{"where the unplugged monitor was", rect{3000, 600, 84, 84}, []rect{laptop}, false},
		{"mostly past the right edge", rect{1410, 400, 84, 84}, []rect{laptop}, false},
		{"under the menu bar", rect{700, -30, 84, 84}, []rect{laptop}, false},
		{"with no screen reported", rect{700, 400, 84, 84}, nil, false},
	}
	for _, tc := range cases {
		if got := onScreen(tc.bubble, tc.screens); got != tc.want {
			t.Errorf("%s: onScreen = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBubbleSideFor_IsTwoThirdsForTheSmallCircle(t *testing.T) {
	if got := bubbleSideFor(false); got != 84 {
		t.Errorf("regular window side = %d; want 84, the 60-point circle and room for its badge", got)
	}
	if got := bubbleSideFor(true); got != 56 {
		t.Errorf("small window side = %d; want 56, the 40-point circle and room for its badge", got)
	}
}

func TestResizedBubble_KeepsTheEdgesNearestTheCircle(t *testing.T) {
	work := rect{X: 0, Y: 25, Width: 1440, Height: 875}
	defaultX, defaultY := defaultBubblePosition(work, 84)
	smallX, smallY := defaultBubblePosition(work, 56)
	cases := []struct {
		name         string
		bubble       rect
		side         int
		wantX, wantY int
	}{
		{"the default place stays the default place", rect{defaultX, defaultY, 84, 84}, 56, smallX, smallY},
		{"and grows back to it", rect{smallX, smallY, 56, 56}, 84, defaultX, defaultY},
		{"upper left keeps its top and left", rect{20, 40, 84, 84}, 56, 20, 40},
		{"upper right keeps its top and right", rect{1340, 40, 84, 84}, 56, 1340 + 28, 40},
		{"lower left keeps its bottom and left", rect{20, 800, 84, 84}, 56, 20, 800 + 28},
	}
	for _, tc := range cases {
		x, y := resizedBubble(tc.bubble, work, tc.side)
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("%s: bubble at (%d, %d), want (%d, %d)", tc.name, x, y, tc.wantX, tc.wantY)
		}
	}
}

func TestShowPanel_UsesTheListedScreensWhenTheBubblesScreenIsUnknown(t *testing.T) {
	s, _ := placedShell(t)
	s.bubble.(*fakeWindow).screenless = true

	s.togglePanel()

	panel := s.panel.(*fakeWindow)
	if !panel.IsVisible() || panel.moves != 1 {
		t.Errorf("list visible %v, placed %d times; want it shown beside the circle", panel.IsVisible(), panel.moves)
	}
}
