# GPS lifecycle

scoped correction; quality, session-race, Android policy, field and battery acceptance remain open.

- B-BR-DRV-09/10: GPS starts only on explicit request for the current running
  shift and captured owner. A duplicate start cannot add a listener or restart
  its accumulator. Stop releases this service's listener and persists its
  aggregate before ending the foreground service.
- B-BR-DRV-07/09: persist an owner-scoped incomplete checkpoint before fixes;
  interrupted recording remains incomplete after process death and never
  restarts automatically. Store no coordinates. A final observation remains
  incomplete with a gap until a reviewed versioned quality policy exists.
- B-BR-DRV-10/18: account change detected on a location callback stops
  collection; the initial owner owns the checkpoint and aggregate. It is never
  rebound to a subsequent user. Session checks do not serialize account changes
  with Room or detect changes while no callback arrives. A separate auth/session
  authority is required to close those gaps. Permission loss without a callback
  can also remain undetected. Pausing, ending or entering recovery of the
  manual shift now signals an already running service to stop without
  launching a new one; a stop requested while the service process is gone
  relies on the incomplete checkpoint. These are open lifecycle obligations,
  not acceptance of B-BR-DRV-10.
- BUC-DRV-03: denial, disabled provider, duplicate start, repeated stop,
  persistence failure and owner switch must not create a complete estimate or
  leave an intentionally retained listener. Manual shifts remain independent.

## iOS session lifecycle

A running shift can acquire the iOS location provider only after the driver enables GPS for that shift and approves When In Use permission. The provider displays the background location indicator while the opted-in lease is active. It shares the provider with in-app navigation without sharing stored data. Permission loss, shift pause, end, recovery, logout, account deletion and origin switch stop the driver's lease. Reopening the app shows its protected incomplete checkpoint and requires a new explicit start.

The current in-memory samples are used only to derive metres. The app persists metres, the observation window and its coverage and quality state in the account/origin scope. A denied permission, gap, rejected fix or interrupted run cannot become complete coverage. GPS coordinates and traces are absent from persistence and the Abastevo wire.

Physical-device acceptance includes permission loss while backgrounded, lock screen, navigation combined with a driving shift, force termination and restart, battery impact and owner switching.
