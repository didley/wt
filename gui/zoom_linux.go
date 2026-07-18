//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

static void reset_zoom(WebKitWebView *view) {
	if (webkit_web_view_get_zoom_level(view) != 1.0) {
		webkit_web_view_set_zoom_level(view, 1.0);
	}
}

static void on_zoom_notify(GObject *obj, GParamSpec *pspec, gpointer user_data) {
	reset_zoom(WEBKIT_WEB_VIEW(obj));
}

// find_and_guard walks a widget subtree looking for the WebKitWebView and,
// once found, pins its zoom level at 1.0 for the life of the window.
static void find_and_guard(GtkWidget *widget) {
	if (WEBKIT_IS_WEB_VIEW(widget)) {
		g_signal_connect(widget, "notify::zoom-level", G_CALLBACK(on_zoom_notify), NULL);
		reset_zoom(WEBKIT_WEB_VIEW(widget));
		return;
	}
	if (GTK_IS_CONTAINER(widget)) {
		GList *children = gtk_container_get_children(GTK_CONTAINER(widget));
		for (GList *l = children; l != NULL; l = l->next) {
			find_and_guard(GTK_WIDGET(l->data));
		}
		g_list_free(children);
	}
}

static gboolean guard_zoom_idle(gpointer user_data) {
	GList *windows = gtk_window_list_toplevels();
	for (GList *l = windows; l != NULL; l = l->next) {
		find_and_guard(GTK_WIDGET(l->data));
	}
	g_list_free(windows);
	return G_SOURCE_REMOVE;
}

static void schedule_disable_zoom() {
	g_idle_add(guard_zoom_idle, NULL);
}
*/
import "C"

// disableWebviewZoom pins the WebKitWebView's zoom level at 1.0 so
// ctrl-scroll, ctrl-+/-/0, and trackpad pinch gestures can't zoom the
// window. WebKitGTK handles all of these natively at the widget level —
// before the DOM ever sees a wheel or keydown event — so this can't be
// done from frontend/dist/app.js; it has to reach into GTK directly.
//
// Wails doesn't expose the webview widget it creates for Linux, so this
// finds it by walking gtk_window_list_toplevels() instead. The search
// runs via g_idle_add, which is thread-safe to call at any point and
// fires once GTK's main loop starts pumping events — so call ordering
// relative to window creation doesn't matter.
func disableWebviewZoom() {
	C.schedule_disable_zoom()
}
