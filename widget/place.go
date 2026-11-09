package main

// rect is a rectangle on screen, in the points the window system uses,
// its origin at the top left.
type rect struct {
	X, Y, Width, Height int
}

// corner is the corner of the panel that sits nearest the bubble. The
// panel grows out of it when it opens.
type corner string

const (
	cornerTopLeft     corner = "top-left"
	cornerTopRight    corner = "top-right"
	cornerBottomLeft  corner = "bottom-left"
	cornerBottomRight corner = "bottom-right"
)

// panelGap is the space between the bubble and the panel.
const panelGap = 4

// placePanel puts a panel of the given size beside the bubble, on the
// side with more room, and keeps it inside the screen's work area. A
// bubble in the right half gets the panel on its left; a bubble in the
// lower half gets the panel's bottom edge level with its own.
func placePanel(bubble, work rect, width, height int) (x, y int, nearest corner) {
	bubbleOnRight := bubble.X+bubble.Width/2 > work.X+work.Width/2
	bubbleBelow := bubble.Y+bubble.Height/2 > work.Y+work.Height/2

	x = bubble.X + bubble.Width + panelGap
	if bubbleOnRight {
		x = bubble.X - panelGap - width
	}
	y = bubble.Y
	if bubbleBelow {
		y = bubble.Y + bubble.Height - height
	}
	x = clamp(x, work.X, work.X+work.Width-width)
	y = clamp(y, work.Y, work.Y+work.Height-height)

	corners := map[[2]bool]corner{
		{false, false}: cornerTopLeft,
		{true, false}:  cornerTopRight,
		{false, true}:  cornerBottomLeft,
		{true, true}:   cornerBottomRight,
	}
	return x, y, corners[[2]bool{bubbleOnRight, bubbleBelow}]
}

// clamp keeps value between low and high. A work area smaller than the
// panel has high below low; the panel then starts at low.
func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	return min(max(value, low), high)
}

// defaultBubblePosition is the lower right corner of the work area,
// where the bubble starts before the person has moved it.
func defaultBubblePosition(work rect, side int) (x, y int) {
	const margin = 24
	return work.X + work.Width - side - margin, work.Y + work.Height - side - margin
}
