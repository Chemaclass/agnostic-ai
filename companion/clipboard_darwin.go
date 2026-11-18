//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

static char *companionClipboardHTML(void) {
	@autoreleasepool {
		NSString *html = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeHTML];
		if (html == nil) {
			return NULL;
		}
		const char *bytes = [html UTF8String];
		return bytes == NULL ? NULL : strdup(bytes);
	}
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// clipboardHTML reads the HTML flavor a browser puts beside the copied
// text. Wails reads only the text. Both are read on the main thread, as
// AppKit wants.
func clipboardHTML() string {
	return application.InvokeSyncWithResult(func() string {
		html := C.companionClipboardHTML()
		if html == nil {
			return ""
		}
		defer C.free(unsafe.Pointer(html))
		return C.GoString(html)
	})
}
