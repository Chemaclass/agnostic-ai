//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

static BOOL companionAcceptsFirstMouse(id self, SEL _cmd, NSEvent *event) {
	return YES;
}

// A view that does not accept the first mouse only brings its window
// forward on a click made while another app is in front, and the click
// itself is thrown away. The companion's windows are web views, so the
// answer is set on the web view class, for every web view of this process.
static void companionTakeFirstClick(void) {
	class_replaceMethod([WKWebView class], @selector(acceptsFirstMouse:), (IMP)companionAcceptsFirstMouse, "c@:@");
}
*/
import "C"

// takeFirstClick makes a click on the companion count even while
// another app is in front. Without it macOS spends that click on
// bringing the window forward, and the circle needs a second one.
func takeFirstClick() {
	C.companionTakeFirstClick()
}
