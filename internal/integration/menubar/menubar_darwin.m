#import <Cocoa/Cocoa.h>
#include "menubar_darwin.h"

// Implemented in Go; the numbers match the action constants in menubar.go.
extern void nottedMenuBarAction(int action);

@interface NottedMenuTarget : NSObject
- (void)pick:(NSMenuItem *)sender;
@end

@implementation NottedMenuTarget
- (void)pick:(NSMenuItem *)sender {
	nottedMenuBarAction((int)sender.tag);
}
@end

static NSStatusItem *statusItem;
static NottedMenuTarget *target;
static BOOL isRecording;

static NSMenuItem *menuItem(NSString *title, NSInteger tag) {
	NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:title action:@selector(pick:) keyEquivalent:@""];
	mi.target = target;
	mi.tag = tag;
	return mi;
}

// render sets the icon and rebuilds the menu for the current state. Main thread only.
static void render(void) {
	if (statusItem == nil) {
		return;
	}
	NSString *symbol = isRecording ? @"record.circle.fill" : @"waveform";
	NSString *label = isRecording ? @"Notted — recording" : @"Notted";
	NSImage *image = [NSImage imageWithSystemSymbolName:symbol accessibilityDescription:label];
	image.template = YES;
	statusItem.button.image = image;
	statusItem.button.toolTip = label;

	NSMenu *menu = [[NSMenu alloc] init];
	if (isRecording) {
		[menu addItem:menuItem(@"Stop recording", 1)];
	} else {
		[menu addItem:menuItem(@"Start recording", 0)];
	}
	[menu addItem:menuItem(@"Open Notted", 2)];
	[menu addItem:[NSMenuItem separatorItem]];
	[menu addItem:menuItem(@"Quit Notted", 3)];
	statusItem.menu = menu;
}

void NottedMenuBarInstall(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (statusItem != nil) {
			return;
		}
		target = [[NottedMenuTarget alloc] init];
		statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
		render();
	});
}

void NottedMenuBarSetRecording(int recording) {
	dispatch_async(dispatch_get_main_queue(), ^{
		isRecording = recording != 0;
		render();
	});
}
