#import <Cocoa/Cocoa.h>
#include "menubar_darwin.h"

// Implemented in Go; the numbers match the action constants in menubar.go.
extern void phononMenuBarAction(int action);

@interface PhononMenuTarget : NSObject
- (void)pick:(NSMenuItem *)sender;
@end

@implementation PhononMenuTarget
- (void)pick:(NSMenuItem *)sender {
	phononMenuBarAction((int)sender.tag);
}
@end

static NSStatusItem *statusItem;
static PhononMenuTarget *target;
static BOOL isRecording;

static NSMenuItem *menuItem(NSString *title, NSInteger tag) {
	NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:title action:@selector(pick:) keyEquivalent:@""];
	mi.target = target;
	mi.tag = tag;
	return mi;
}

// The five sound bars of build/appicon.svg (concept P1 Waveform), in its 64-unit glyph space
// (y down): x, top, bottom. Strokes are 4 units wide with round caps, as in the app icon.
static const CGFloat bars[][3] = {
	{19, 28, 36}, {25.5, 24, 40}, {32, 21, 43}, {38.5, 25, 39}, {45, 28, 36},
};
static const CGFloat barWidth = 4;
static const NSRect iconBox = {{13, 16}, {39, 32}};
static const CGFloat dotX = 47.5, dotY = 20.5, dotR = 3.8;

// icon draws the bars as a template image, with a particle dot above the last bar while
// recording. Both states share one frame, so the bars do not move when the dot appears.
static NSImage *icon(BOOL recording, NSString *label) {
	CGFloat scale = 18.0 / iconBox.size.height;
	NSSize size = NSMakeSize(iconBox.size.width * scale, 18.0);
	NSImage *image = [NSImage imageWithSize:size flipped:YES drawingHandler:^BOOL(NSRect rect) {
		NSAffineTransform *t = [NSAffineTransform transform];
		[t scaleBy:scale];
		[t translateXBy:-iconBox.origin.x yBy:-iconBox.origin.y];
		[t concat];
		[[NSColor blackColor] set];
		NSBezierPath *path = [NSBezierPath bezierPath];
		path.lineWidth = barWidth;
		path.lineCapStyle = NSLineCapStyleRound;
		size_t n = sizeof(bars) / sizeof(bars[0]);
		for (size_t i = 0; i < n; i++) {
			[path moveToPoint:NSMakePoint(bars[i][0], bars[i][1])];
			[path lineToPoint:NSMakePoint(bars[i][0], bars[i][2])];
		}
		[path stroke];
		if (recording) {
			[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(dotX - dotR, dotY - dotR, 2 * dotR, 2 * dotR)] fill];
		}
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
	NSString *label = isRecording ? @"Phonon — recording" : @"Phonon";
	NSImage *image = icon(isRecording, label);
	statusItem.button.image = image;
	statusItem.button.toolTip = label;

	NSMenu *menu = [[NSMenu alloc] init];
	if (isRecording) {
		[menu addItem:menuItem(@"Stop recording", 1)];
	} else {
		[menu addItem:menuItem(@"Start recording", 0)];
	}
	[menu addItem:menuItem(@"Open Phonon", 2)];
	[menu addItem:[NSMenuItem separatorItem]];
	[menu addItem:menuItem(@"Quit Phonon", 3)];
	statusItem.menu = menu;
}

void PhononMenuBarInstall(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (statusItem != nil) {
			return;
		}
		target = [[PhononMenuTarget alloc] init];
		statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
		render();
	});
}

void PhononMenuBarSetRecording(int recording) {
	dispatch_async(dispatch_get_main_queue(), ^{
		isRecording = recording != 0;
		render();
	});
}
