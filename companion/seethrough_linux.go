//go:build linux && cgo

package main

/*
#cgo pkg-config: gtk4

#include <gtk/gtk.h>

// GTK 4 paints every window with the theme's background, under the web
// view. The window library leaves that to CSS and adds none, so the
// circle sat on an opaque square.
static void companionClearFramelessBackgrounds(void) {
	GdkDisplay *display = gdk_display_get_default();
	if (display == NULL) {
		return;
	}
	GtkCssProvider *provider = gtk_css_provider_new();
	gtk_css_provider_load_from_string(provider, "window.wails-frameless { background: transparent; }");
	gtk_style_context_add_provider_for_display(display, GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
	g_object_unref(provider);
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// seeThrough lets the frameless windows show what lies behind them.
func seeThrough() {
	application.InvokeAsync(func() { C.companionClearFramelessBackgrounds() })
}
