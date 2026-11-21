// https://developer.apple.com/library/archive/qa/qa1340/_index.html

#include <ctype.h>
#include <stdio.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdlib.h>

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

// Only the listener thread accesses the previous power-source snapshot.
typedef struct {
    int onAC;
    CFDictionaryRef adapter;
} PowerConnection;

static void powerSourceCallback(void *context)
{
    PowerConnection *previous = context;
    CFTypeRef info = IOPSCopyPowerSourcesInfo();
    if (info == NULL) return;
    CFStringRef source = IOPSGetProvidingPowerSourceType(info);
    if (source != NULL) {
        int onAC = CFEqual(source, CFSTR(kIOPSACPowerValue));
        CFDictionaryRef adapter = IOPSCopyExternalPowerAdapterDetails();
        // Replacing MagSafe with USB-C can leave the Mac on AC throughout.
        if (onAC != previous->onAC || (adapter == NULL) != (previous->adapter == NULL) ||
            (adapter != NULL && !CFEqual(adapter, previous->adapter))) {
            powerConnectionChangedCallback();
        }
        if (previous->adapter != NULL) CFRelease(previous->adapter);
        previous->adapter = adapter;
        previous->onAC = onAC;
    }
    CFRelease(info);
}

// Stop can arrive before the listener starts its run loop. Signal a run-loop
// source so that either ordering executes the stop on the listener thread.
static pthread_mutex_t listenerMutex = PTHREAD_MUTEX_INITIALIZER;
static CFRunLoopRef listenerLoop;
static CFRunLoopSourceRef stopSource;
static bool stopRequested;

static void stopCallback(void *context)
{
    CFRunLoopStop(CFRunLoopGetCurrent());
}

int ListenNotifications()
{
    root_port = IORegisterForSystemPower(NULL, &notifyPortRef, sleepCallBack, &notifierObject);
    if (root_port == 0) {
        printf("IORegisterForSystemPower failed\n");
        return 1;
    }

    CFRunLoopRef loop = CFRunLoopGetCurrent();
    CFRunLoopSourceRef sleepSource = IONotificationPortGetRunLoopSource(notifyPortRef);
    CFRunLoopAddSource(loop, sleepSource, kCFRunLoopCommonModes);

    PowerConnection previous = { .onAC = -1, .adapter = NULL };
    CFRunLoopSourceRef powerSource = IOPSNotificationCreateRunLoopSource(powerSourceCallback, &previous);
    if (powerSource != NULL) {
        CFRunLoopAddSource(loop, powerSource, kCFRunLoopCommonModes);
        powerSourceCallback(&previous);
    }

    CFRunLoopSourceContext context = { .perform = stopCallback };
    CFRunLoopSourceRef stop = CFRunLoopSourceCreate(NULL, 0, &context);
    if (stop != NULL) {
        CFRunLoopAddSource(loop, stop, kCFRunLoopCommonModes);
        pthread_mutex_lock(&listenerMutex);
        listenerLoop = loop;
        stopSource = stop;
        if (stopRequested) CFRunLoopSourceSignal(stop);
        pthread_mutex_unlock(&listenerMutex);
        CFRunLoopRun();
        pthread_mutex_lock(&listenerMutex);
        listenerLoop = NULL;
        stopSource = NULL;
        stopRequested = false;
        pthread_mutex_unlock(&listenerMutex);
        CFRunLoopRemoveSource(loop, stop, kCFRunLoopCommonModes);
        CFRelease(stop);
    }

    if (powerSource != NULL) {
        CFRunLoopRemoveSource(loop, powerSource, kCFRunLoopCommonModes);
        CFRelease(powerSource);
    }
    if (previous.adapter != NULL) CFRelease(previous.adapter);
    CFRunLoopRemoveSource(loop, sleepSource, kCFRunLoopCommonModes);
    IODeregisterForSystemPower(&notifierObject);
    IOServiceClose(root_port);
    IONotificationPortDestroy(notifyPortRef);
    return stop == NULL;
}

int StopListeningNotifications()
{
    pthread_mutex_lock(&listenerMutex);
    stopRequested = true;
    if (stopSource != NULL) {
        CFRunLoopSourceSignal(stopSource);
        CFRunLoopWakeUp(listenerLoop);
    }
    pthread_mutex_unlock(&listenerMutex);
    return 0;
}
