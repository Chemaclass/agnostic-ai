//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

extern void companionPointerChanged(int over);

@interface CompanionPointerWatch : NSResponder
@end

@implementation CompanionPointerWatch
- (void)mouseEntered:(NSEvent *)event {
	companionPointerChanged(1);
}
- (void)mouseExited:(NSEvent *)event {
	companionPointerChanged(0);
}
@end

void companionWatchPointer(void *window) {
	dispatch_async(dispatch_get_main_queue(), ^{
		static CompanionPointerWatch *watch;
		if (watch == nil) {
			watch = [[CompanionPointerWatch alloc] init];
		}
		NSView *view = [(NSWindow *)window contentView];
		NSTrackingArea *area = [[NSTrackingArea alloc] initWithRect:NSZeroRect
			options:NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect
			owner:watch
			userInfo:nil];
		[view addTrackingArea:area];
		[area release];
	});
}
*/
import "C"
