// https://developer.apple.com/library/archive/qa/qa1340/_index.html

#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <pthread.h>

#include <mach/mach_init.h>
#include <mach/mach_interface.h>
#include <mach/mach_port.h>

#include <IOKit/IOMessage.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <IOKit/ps/IOPowerSources.h>
#include <IOKit/ps/IOPSKeys.h>

#include "hook.h"

io_connect_t root_port; // a reference to the Root Power Domain IOService
// notification port allocated by IORegisterForSystemPower
IONotificationPortRef notifyPortRef;
// notifier object, used to deregister later
io_object_t notifierObject;
long gMessageArgument;
static CFRunLoopSourceRef powerSourceNotification;
static CFRunLoopRef notificationRunLoop;
static CFDictionaryRef previousAdapterDetails;
static pthread_mutex_t powerSourceMutex = PTHREAD_MUTEX_INITIALIZER;
static bool powerSourceStopped;

static void powerSourceCallback(void *context) {
    pthread_mutex_lock(&powerSourceMutex);
    if (powerSourceStopped) { pthread_mutex_unlock(&powerSourceMutex); return; }
    CFTypeRef info = IOPSCopyPowerSourcesInfo();
    if (info == NULL) { pthread_mutex_unlock(&powerSourceMutex); return; }
    CFStringRef source = IOPSGetProvidingPowerSourceType(info);
    if (source != NULL) powerSourceChangedCallback(CFEqual(source, CFSTR(kIOPSACPowerValue)) ? 1 : 0);
    // Switching between MagSafe and USB-C can keep the Mac on AC power.
    // Treat an adapter-identity/negotiation change as a fresh connection too.
    CFDictionaryRef adapter = IOPSCopyExternalPowerAdapterDetails();
    Boolean changed = (adapter == NULL) != (previousAdapterDetails == NULL);
    if (adapter != NULL && previousAdapterDetails != NULL && !CFEqual(adapter, previousAdapterDetails)) changed = true;
    if (changed) powerAdapterChangedCallback();
    if (previousAdapterDetails != NULL) CFRelease(previousAdapterDetails);
    previousAdapterDetails = adapter;
    CFRelease(info);
    pthread_mutex_unlock(&powerSourceMutex);
}

int AllowPowerChange()
{
    return IOAllowPowerChange(root_port, gMessageArgument);
}

int CancelPowerChange()
{
    return IOCancelPowerChange(root_port, gMessageArgument);
}

void sleepCallBack(void* refCon, io_service_t service, natural_t messageType, void* messageArgument)
{
    gMessageArgument = (long)messageArgument;

    switch (messageType) {
    case kIOMessageCanSystemSleep:
        /* Idle sleep is about to kick in. This message will not be sent for forced sleep.
            Applications have a chance to prevent sleep by calling IOCancelPowerChange.
            Most applications should not prevent idle sleep.

            Power Management waits up to 30 seconds for you to either allow or deny idle
            sleep. If you don't acknowledge this power change by calling either
            IOAllowPowerChange or IOCancelPowerChange, the system will wait 30
            seconds then go to sleep.
        */

        // Cancel idle sleep
        // IOCancelPowerChange( root_port, (long)messageArgument );
        //  Allow idle sleep
        // IOAllowPowerChange(root_port, (long)messageArgument);

        canSystemSleepCallback();

        break;

    case kIOMessageSystemWillSleep:
        /* The system WILL go to sleep. If you do not call IOAllowPowerChange or
            IOCancelPowerChange to acknowledge this message, sleep will be
            delayed by 30 seconds.

            NOTE: If you call IOCancelPowerChange to deny sleep it returns
            kIOReturnSuccess, however the system WILL still go to sleep.
        */

        systemWillSleepCallback();

        break;

    case kIOMessageSystemWillPowerOn:
        // System has started the wake up process...

        systemWillPowerOnCallback();

        break;

    case kIOMessageSystemHasPoweredOn:
        // System has finished waking up...

        systemHasPoweredOnCallback();

        break;

    default:
        break;
    }
}

int ListenNotifications()
{
    // this parameter is passed to the callback
    void* refCon = NULL;

    // register to receive system sleep notifications
    root_port = IORegisterForSystemPower(refCon, &notifyPortRef, sleepCallBack, &notifierObject);
    if (root_port == 0) {
        printf("IORegisterForSystemPower failed\n");
        return 1;
    }

    // add the notification port to the application runloop
    CFRunLoopAddSource(CFRunLoopGetCurrent(),
        IONotificationPortGetRunLoopSource(notifyPortRef), kCFRunLoopCommonModes);
    notificationRunLoop = CFRunLoopGetCurrent();
    CFRetain(notificationRunLoop);
    powerSourceStopped = false;
    powerSourceNotification = IOPSNotificationCreateRunLoopSource(powerSourceCallback, NULL);
    if (powerSourceNotification != NULL) CFRunLoopAddSource(notificationRunLoop, powerSourceNotification, kCFRunLoopCommonModes);
    powerSourceCallback(NULL);

    // Start the run loop to receive sleep notifications.
    CFRunLoopRun();

    // Not reached, CFRunLoopRun doesn't return in this case.
    return 0;
}

int StopListeningNotifications()
{
    // remove the sleep notification port from the application runloop
    if (notificationRunLoop == NULL) return 0;
    pthread_mutex_lock(&powerSourceMutex);
    powerSourceStopped = true;
    if (powerSourceNotification != NULL) {
        CFRunLoopRemoveSource(notificationRunLoop, powerSourceNotification, kCFRunLoopCommonModes);
        CFRelease(powerSourceNotification);
        powerSourceNotification = NULL;
    }
    CFRunLoopRemoveSource(notificationRunLoop,
       IONotificationPortGetRunLoopSource(notifyPortRef),
       kCFRunLoopCommonModes);

    // deregister for system sleep notifications
    IODeregisterForSystemPower(&notifierObject);

    // IORegisterForSystemPower implicitly opens the Root Power Domain IOService
    // so we close it here
    IOServiceClose(root_port);

    // destroy the notification port allocated by IORegisterForSystemPower
    IONotificationPortDestroy(notifyPortRef);
    if (previousAdapterDetails != NULL) { CFRelease(previousAdapterDetails); previousAdapterDetails = NULL; }
    pthread_mutex_unlock(&powerSourceMutex);
    CFRunLoopStop(notificationRunLoop);
    CFRelease(notificationRunLoop);
    notificationRunLoop = NULL;

    return 0;
}
