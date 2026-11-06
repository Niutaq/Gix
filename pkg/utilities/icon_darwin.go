package utilities

/*
#cgo LDFLAGS: -framework Cocoa -framework AppKit
#import <Cocoa/Cocoa.h>
#import <stdlib.h>

void setDockIconPNG(const void *data, int len) {
    @autoreleasepool {
        NSData *nsData = [NSData dataWithBytes:data length:len];
        NSImage *img = [[NSImage alloc] initWithData:nsData];
        if (img == nil) {
            return;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            // go run ./cmd/gix/main.go otherwise shows as "main" in Dock / menu.
            [[NSProcessInfo processInfo] setProcessName:@"Gix"];
            [NSApp setApplicationIconImage:img];
            NSMenu *mainMenu = [NSApp mainMenu];
            if (mainMenu != nil && mainMenu.numberOfItems > 0) {
                NSMenuItem *appItem = [mainMenu itemAtIndex:0];
                if (appItem != nil) {
                    [appItem setTitle:@"Gix"];
                    NSMenu *appMenu = [appItem submenu];
                    if (appMenu != nil) {
                        [appMenu setTitle:@"Gix"];
                    }
                }
            }
        });
    }
}
*/
import "C"

// SetDockIcon sets the macOS Dock icon + process name (works under go run / Ghostty).
func SetDockIcon(png []byte) {
	if len(png) == 0 {
		return
	}
	p := C.CBytes(png)
	defer C.free(p)
	C.setDockIconPNG(p, C.int(len(png)))
}
