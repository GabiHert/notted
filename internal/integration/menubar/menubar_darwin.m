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

// The serif N and the dot of build/appicon.svg, in its 1024-unit space (y down).
static const CGFloat nPoints[][2] = {
	{335.85, 689.14}, {335.85, 334.91}, {280.16, 317.31}, {280.16, 309.13}, {412.11, 309.13},
	{638.82, 623.86}, {623.86, 671.55}, {623.86, 329.81}, {570.46, 317.82}, {570.46, 309.13},
	{692.33, 309.13}, {692.33, 317.82}, {643.64, 329.81}, {643.64, 714.24}, {618.03, 714.24},
	{347.23, 339.44}, {354.74, 330.48}, {354.74, 689.14}, {408.13, 701.13}, {408.13, 709.76},
	{287.16, 709.76}, {287.16, 701.13},
};
static const NSRect iconBox = {{270, 210}, {550, 510}};
static const CGFloat dotX = 763.3, dotY = 268.9, dotR = 51.1;

// icon draws the N as a template image, with the app icon's dot while recording.
// Both states share one frame, so the N does not move when the dot appears.
static NSImage *icon(BOOL recording, NSString *label) {
	CGFloat scale = 18.0 / iconBox.size.height;
	NSSize size = NSMakeSize(iconBox.size.width * scale, 18.0);
	NSImage *image = [NSImage imageWithSize:size flipped:YES drawingHandler:^BOOL(NSRect rect) {
		NSAffineTransform *t = [NSAffineTransform transform];
		[t scaleBy:scale];
		[t translateXBy:-iconBox.origin.x yBy:-iconBox.origin.y];
		NSBezierPath *path = [NSBezierPath bezierPath];
		size_t n = sizeof(nPoints) / sizeof(nPoints[0]);
		[path moveToPoint:NSMakePoint(nPoints[0][0], nPoints[0][1])];
		for (size_t i = 1; i < n; i++) {
			[path lineToPoint:NSMakePoint(nPoints[i][0], nPoints[i][1])];
		}
		[path closePath];
		if (recording) {
			[path appendBezierPathWithOvalInRect:NSMakeRect(dotX - dotR, dotY - dotR, 2 * dotR, 2 * dotR)];
		}
		[path transformUsingAffineTransform:t];
		[[NSColor blackColor] setFill];
		[path fill];
		return YES;
	}];
	image.template = YES;
	image.accessibilityDescription = label;
	return image;
}

// render sets the icon and rebuilds the menu for the current state. Main thread only.
static void render(void) {
	if (statusItem == nil) {
		return;
	}
	NSString *label = isRecording ? @"Notted — recording" : @"Notted";
	NSImage *image = icon(isRecording, label);
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
