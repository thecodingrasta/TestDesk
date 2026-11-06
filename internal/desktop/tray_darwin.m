//go:build darwin

#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>
#import <stdatomic.h>

#import "tray_darwin.h"

extern void testdeskTrayReady(void);
extern void testdeskTrayMenuSelected(int command);

static NSStatusItem *testdeskStatusItem;
static id testdeskMenuTarget;
static atomic_bool testdeskTrayStopped;

@interface TestDeskMenuTarget : NSObject
- (void)selectItem:(NSMenuItem *)sender;
@end

@implementation TestDeskMenuTarget
- (void)selectItem:(NSMenuItem *)sender {
    testdeskTrayMenuSelected((int)sender.tag);
}
@end

static void addItem(NSMenu *menu, NSString *title, NSInteger command) {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title
                                                  action:@selector(selectItem:)
                                           keyEquivalent:@""];
    item.tag = command;
    item.target = testdeskMenuTarget;
    [menu addItem:item];
}

void testdeskTrayStart(const void *icon, int length) {
    if (icon == NULL || length <= 0) {
        return;
    }
    atomic_store(&testdeskTrayStopped, false);
    NSData *iconData = [NSData dataWithBytes:icon length:(NSUInteger)length];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (atomic_load(&testdeskTrayStopped) || testdeskStatusItem != nil) {
            return;
        }
        NSImage *image = [[NSImage alloc] initWithData:iconData];
        if (image == nil) {
            return;
        }
        image.size = NSMakeSize(18, 18);
        NSStatusItem *item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        if (item == nil || item.button == nil) {
            return;
        }
        testdeskMenuTarget = [[TestDeskMenuTarget alloc] init];
        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"TestDesk"];
        addItem(menu, @"Open Test Manager", 1);
        addItem(menu, @"Show Controller", 2);
        addItem(menu, @"Projects / Add Project", 3);
        addItem(menu, @"Refresh Tests", 4);
        [menu addItem:[NSMenuItem separatorItem]];
        addItem(menu, @"Pause Queue", 5);
        addItem(menu, @"Resume Queue", 6);
        [menu addItem:[NSMenuItem separatorItem]];
        addItem(menu, @"Exit TestDesk", 7);
        item.button.image = image;
        item.button.toolTip = @"TestDesk - test lifecycle manager";
        item.menu = menu;
        testdeskStatusItem = item;
        testdeskTrayReady();
    });
}

void testdeskTrayStop(void) {
    atomic_store(&testdeskTrayStopped, true);
    dispatch_async(dispatch_get_main_queue(), ^{
        if (testdeskStatusItem != nil) {
            [[NSStatusBar systemStatusBar] removeStatusItem:testdeskStatusItem];
            testdeskStatusItem = nil;
        }
        testdeskMenuTarget = nil;
    });
}
