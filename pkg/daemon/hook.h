#pragma once

extern void canSystemSleepCallback();
extern void systemWillSleepCallback();
extern void systemWillPowerOnCallback();
extern void systemHasPoweredOnCallback();
extern void powerConnectionChangedCallback();

int AllowPowerChange();
int CancelPowerChange();
int ListenNotifications();
int StopListeningNotifications();
