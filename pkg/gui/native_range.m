#import "native_internal.h"

bool batt_show_charge_range(int *upper, int *lower, const char *status, const char *led) {
    @autoreleasepool {
        NSAlert *alert = [[[NSAlert alloc] init] autorelease];
        alert.messageText = BattLocalized(@"Charge Range");
        alert.informativeText = [NSString stringWithFormat:@"%@\n\n%@\n%@",
            BattLocalized(@"Charging pauses at the upper limit and resumes below the lower limit."),
            [NSString stringWithUTF8String:status], [NSString stringWithUTF8String:led]];
        [alert addButtonWithTitle:BattLocalized(@"Save")];
        [alert addButtonWithTitle:BattLocalized(@"Cancel")];
        NSView *view = [[[NSView alloc] initWithFrame:NSMakeRect(0, 0, 320, 90)] autorelease];
        NSTextField *upperLabel = [NSTextField labelWithString:BattLocalized(@"Upper limit (%)")];
        upperLabel.frame = NSMakeRect(0, 55, 210, 24);
        NSTextField *lowerLabel = [NSTextField labelWithString:BattLocalized(@"Lower limit (%)")];
        lowerLabel.frame = NSMakeRect(0, 15, 210, 24);
        NSTextField *upperField = [[[NSTextField alloc] initWithFrame:NSMakeRect(225, 55, 80, 24)] autorelease];
        NSTextField *lowerField = [[[NSTextField alloc] initWithFrame:NSMakeRect(225, 15, 80, 24)] autorelease];
        upperField.stringValue = [NSString stringWithFormat:@"%d", *upper];
        lowerField.stringValue = [NSString stringWithFormat:@"%d", *lower];
        upperField.accessibilityLabel = upperLabel.stringValue;
        lowerField.accessibilityLabel = lowerLabel.stringValue;
        [view addSubview:upperLabel]; [view addSubview:lowerLabel];
        [view addSubview:upperField]; [view addSubview:lowerField];
        alert.accessoryView = view;
        [[NSApplication sharedApplication] activateIgnoringOtherApps:YES];
        [alert.window makeFirstResponder:upperField];
        NSRegularExpression *digits = [NSRegularExpression regularExpressionWithPattern:@"^[0-9]{1,3}$" options:0 error:NULL];
        while ([alert runModal] == NSAlertFirstButtonReturn) {
            BOOL wholeUpper = [digits numberOfMatchesInString:upperField.stringValue options:0 range:NSMakeRange(0, upperField.stringValue.length)] == 1;
            BOOL wholeLower = [digits numberOfMatchesInString:lowerField.stringValue options:0 range:NSMakeRange(0, lowerField.stringValue.length)] == 1;
            NSInteger u = upperField.integerValue, l = lowerField.integerValue;
            if (wholeUpper && wholeLower && u <= 100 && l >= 10 && l < u) {
                *upper = (int)u; *lower = (int)l; return true;
            }
            batt_show_alert("Invalid charge range", "Use whole percentages satisfying 10 <= lower < upper <= 100.");
        }
        return false;
    }
}
