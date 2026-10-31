#pragma once

extern void canSystemSleepCallback();
extern void systemWillSleepCallback();
extern void systemWillPowerOnCallback();
extern void systemHasPoweredOnCallback();
extern void powerSourceChangedCallback(int connected);
extern void powerAdapterChangedCallback();

int AllowPowerChange();
int CancelPowerChange();
int ListenNotifications();
int StopListeningNotifications();
