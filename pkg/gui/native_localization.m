#import "native_internal.h"
#include <stdlib.h>
#include <string.h>

static NSDictionary *BattMessages;

NSString *BattLocalized(NSString *message) {
    NSString *translated = [BattMessages objectForKey:message];
    return translated ?: message;
}

char *batt_preferred_language(void) {
    @autoreleasepool {
        return strdup(([NSLocale preferredLanguages].firstObject ?: @"en").UTF8String);
    }
}

void batt_configure_localization(const char *json) {
    @autoreleasepool {
        NSData *data = [[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding];
        NSDictionary *messages = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
        [BattMessages release];
        BattMessages = [messages copy];
    }
}
