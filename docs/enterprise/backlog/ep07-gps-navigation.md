# EP07 — Tracking, routes and navigation

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
sequencing decision.

The existing tracking specification governs detailed defaults, quality and platform behavior.

### EMP-GPS-007 — Live position projection with staleness rules
- **Meta:** module GPS · phase F5 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Project the newest permitted live fix with freshness at read time.
- **Rules and use cases:** BR-GPS-030, BR-GPS-031, BR-GPS-032, BR-GPS-039
- **Depends on:** EMP-GPS-004
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: scoped latest position and age · out: showing old data as current
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** An out-of-order fix cannot replace a newer one and every read carries age and quality.
- **Acceptance:** An out-of-order fix cannot replace a newer one and every read carries age and quality.
- **Negative cases:** Stale fix labelled live, cross-org position or read ignoring expired authority.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-GPS-008 — Panel: live operations map
- **Meta:** module GPS · phase F5 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Show authorized operations on the approved panel map.
- **Rules and use cases:** BR-GPS-031, BR-GPS-032, BR-GPS-033, BR-TRUTH-006
- **Depends on:** EMP-GPS-007, EMP-ROUTE-001, EMP-GPS-012
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: live view with freshness and accuracy · out: unconditional location access
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A stale position is visibly stale and protected reads are audited.
- **Acceptance:** A stale position is visibly stale and protected reads are audited.
- **Negative cases:** Foreign position, stale view shown as live or unavailable data fabricated.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-GPS-009 — Panel: trip track replay aligned with logbook events
- **Meta:** module GPS · phase F5 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Replay an authorized retained track alongside trip events.
- **Rules and use cases:** BR-GPS-024, BR-GPS-026, BR-GPS-033, BR-GPS-035
- **Depends on:** EMP-GPS-005, EMP-TRIP-006, EMP-ROUTE-001
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: scope, gaps and event alignment · out: reconstructing deleted points
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The view preserves gaps and distinguishes collected time from receipt.
- **Acceptance:** The view preserves gaps and distinguishes collected time from receipt.
- **Negative cases:** Foreign trip, expired raw points, silent interpolation or unaudited location read.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-GPS-011 — Track retention: raw-point purge and summary retention
- **Meta:** module GPS · phase F5 · cut - · priority P1 · type JOB · size S · state TODO
- **Objective:** Purge expired raw location and retain only approved summaries.
- **Rules and use cases:** BR-GPS-035, BR-GPS-036, BR-PRIV-009, BR-PRIV-011
- **Depends on:** EMP-GPS-005
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: reviewed expiry, read-time denial and restore replay · out: inventing a legal period
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Expired fixes cannot be read even during cleanup outage and restore cannot re-expose them.
- **Acceptance:** Expired fixes cannot be read even during cleanup outage and restore cannot re-expose them.
- **Negative cases:** Unapproved duration, derivative retaining precise data or wrong organization purged.
- **Validation:** V-SQL, V-JOB, V-IDEM, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-GPS-013 — Geofences with server-side evaluation and alerts
- **Meta:** module GPS · phase F5 · cut - · priority P2 · type JOB · size M · state TODO
- **Objective:** Evaluate approved geofences server-side with quality evidence.
- **Rules and use cases:** BR-GPS-037, BR-GPS-038, BR-GPS-039
- **Depends on:** EMP-GPS-004, EMP-NOTIF-001
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: scoped boundary state and bounded alerts · out: claiming presence from insufficient samples
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A crossing has a documented basis and repeat processing deduplicates it.
- **Acceptance:** A crossing has a documented basis and repeat processing deduplicates it.
- **Negative cases:** Low-quality fix falsely certifies crossing, foreign boundary or duplicate alert.
- **Validation:** V-SQL, V-JOB, V-IDEM, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-GPS-014 — Operational alerts: stale tracker and long stop
- **Meta:** module GPS · phase F5 · cut - · priority P2 · type JOB · size S · state TODO
- **Objective:** Raise scoped stale-tracker and long-stop observations.
- **Rules and use cases:** BR-GPS-015, BR-GPS-016, BR-GPS-031, BR-GPS-038
- **Depends on:** EMP-GPS-007, EMP-NOTIF-001
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: documented freshness and event facts · out: inferring intent or unrestricted notifications
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each alert states its time, basis and scope and respects dispatch bounds.
- **Acceptance:** Each alert states its time, basis and scope and respects dispatch bounds.
- **Negative cases:** Old event reported current, duplicate notice or private location leaked.
- **Validation:** V-SQL, V-JOB, V-IDEM, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-001 — Map components: reuse the Android map engine port and add the panel web map
- **Meta:** module ROUTE · phase F5 · cut - · priority P1 · type INT · size M · state TODO
- **Objective:** Reuse native map ports and admit a reviewed web map boundary.
- **Rules and use cases:** EDR-016, EDR-017, BR-ROUTE-023, BR-ROUTE-024
- **Depends on:** EMP-GOV-007, EMP-PLAT-008, EMP-PLAT-011
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: attribution and replaceable map consumers · out: unreviewed upstream reliance
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each engine or tile source follows its recorded license and approved capability.
- **Acceptance:** Each engine or tile source follows its recorded license and approved capability.
- **Negative cases:** Missing attribution, private traces sent upstream or an unavailable provider with no fallback.
- **Validation:** V-APP, V-WEB, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-002 — Routing port: on-device route computation through the map engine, with a server-side adapter slot
- **Meta:** module ROUTE · phase F5 · cut - · priority P1 · type INT · size M · state TODO
- **Objective:** Compute routes through the existing on-device engine port.
- **Rules and use cases:** BR-ROUTE-009, BR-ROUTE-010, BR-ROUTE-024
- **Depends on:** EMP-GOV-007
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: bounded typed request and attributed result · out: an unapproved server provider
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Routing records engine and profile basis and returns explicit unsupported or unavailable outcomes.
- **Acceptance:** Routing records engine and profile basis and returns explicit unsupported or unavailable outcomes.
- **Negative cases:** Wrong unit, unsupported profile overclaim or provider activation by default.
- **Validation:** V-DOM, V-APP · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-003 — Route plan on missions and trips
- **Meta:** module ROUTE · phase F5 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Store versioned ordered route plans on scoped trips or missions.
- **Rules and use cases:** BR-ROUTE-001, BR-ROUTE-002, BR-ROUTE-003, BR-ROUTE-004, BR-ROUTE-006, BR-ROUTE-008
- **Depends on:** EMP-ROUTE-002, EMP-TRIP-009, EMP-TRIP-010
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: allowed waypoints and source basis · out: overwriting actual movement
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The plan has one owner and revision and remains distinct from travelled evidence.
- **Acceptance:** The plan has one owner and revision and remains distinct from travelled evidence.
- **Negative cases:** Foreign waypoint, too many points, stale overwrite or estimate shown as actual.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-005 — In-app route preview
- **Meta:** module ROUTE · phase F5 · cut - · priority P1 · type APP · size M · state TODO
- **Objective:** Preview a route with attributed estimates and revision.
- **Rules and use cases:** BR-ROUTE-004, BR-ROUTE-005, BR-ROUTE-006, BR-ROUTE-008
- **Depends on:** EMP-ROUTE-001, EMP-ROUTE-003
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: on-device preview and explicit selection · out: claiming actual distance or live traffic
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Users can see the source, estimate and selected version before navigation.
- **Acceptance:** Users can see the source, estimate and selected version before navigation.
- **Negative cases:** Stale preview silently accepted, wrong unit or estimated geometry replacing actual track.
- **Validation:** V-APP · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-006 — Trip-aware turn-by-turn navigation: guidance and tracking as coordinated location consumers
- **Meta:** module ROUTE · phase F5 · cut - · priority P1 · type APP · size L · state TODO
- **Objective:** Coordinate trip guidance and authorized tracking as separate location consumers.
- **Rules and use cases:** BR-ROUTE-013, BR-ROUTE-014, BR-ROUTE-015, BR-GPS-013
- **Depends on:** EMP-ROUTE-005, EMP-GPS-002
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: trip-aware guidance and clean lifecycle · out: a duplicate listener or navigation granting collection authority
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One coordinated source serves the allowed consumers and one consumer stopping does not corrupt the other's state.
- **Acceptance:** One coordinated source serves the allowed consumers and one consumer stopping does not corrupt the other's state.
- **Negative cases:** Hidden tracking from guidance, listener leak, stale trip binding or location after both consumers stop.
- **Validation:** V-APP, V-PRIV, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-007 — Driving-mode experience: quick actions and stop-only detailed entry
- **Meta:** module ROUTE · phase F5 · cut - · priority P1 · type APP · size M · state TODO
- **Objective:** Provide safe quick navigation actions and defer detailed entry to a stop.
- **Rules and use cases:** BR-ROUTE-018, BR-ROUTE-019
- **Depends on:** EMP-ROUTE-006
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: accessible driving view and queued stop-only actions · out: forced detailed data entry while driving
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The driving state permits only the documented quick interactions.
- **Acceptance:** The driving state permits only the documented quick interactions.
- **Negative cases:** A detailed form required during driving, inaccessible stop action or data lost when deferred.
- **Validation:** V-APP · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-008 — Station suggestions along the route
- **Meta:** module ROUTE · phase F5 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Suggest route stations using comparable prices and stated detour basis.
- **Rules and use cases:** BR-ROUTE-020, BR-ROUTE-021, BR-ROUTE-022, BR-SRC-001, BR-SRC-002
- **Depends on:** EMP-ROUTE-002, EMP-FUEL-014
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: scoped route suggestions and source freshness · out: promised savings or paid ranking
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each comparison shows conditions and inputs and leaves incompatible references unranked.
- **Acceptance:** Each comparison shows conditions and inputs and leaves incompatible references unranked.
- **Negative cases:** Mixed payment terms, stale price shown current, paid trust or false savings claim.
- **Validation:** V-DOM, V-APP, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-009 — Navigation degraded and offline behaviour
- **Meta:** module ROUTE · phase F5 · cut - · priority P2 · type APP · size S · state TODO
- **Objective:** Preserve usable guidance when a route or network source is unavailable.
- **Rules and use cases:** BR-ROUTE-017, BR-GPS-005, BR-TRUTH-003
- **Depends on:** EMP-ROUTE-006
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: supported offline route state and explicit degradation · out: invented route certainty
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The trip remains usable and the app explains which data or map region is missing.
- **Acceptance:** The trip remains usable and the app explains which data or map region is missing.
- **Negative cases:** Failure hidden, unavailable routing shown as valid or offline state drops the trip.
- **Validation:** V-APP, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-ROUTE-010 — Engine and adapter replacement drill against the routing and map ports
- **Meta:** module ROUTE · phase F5 · cut - · priority P2 · type TEST · size S · state TODO
- **Objective:** Rehearse map and routing adapter replacement through existing ports.
- **Rules and use cases:** BR-ROUTE-023, BR-ROUTE-024, CX-10
- **Depends on:** EMP-ROUTE-002, EMP-ROUTE-006
- **Read first:** [tracking and routing specification](../modules/gps-navigation.md), [privacy](../privacy-security.md) and [validation](../testing.md)
- **Scope:** in: approved fixture substitution and contract tests · out: new unreviewed dependencies
- **Areas:** existing: native map and location ports, scoped storage and jobs · proposed: reviewed organization tracking and routing consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Consumers retain their typed semantics, attribution and fallback when an admitted adapter changes.
- **Acceptance:** Consumers retain their typed semantics, attribution and fallback when an admitted adapter changes.
- **Negative cases:** Provider-specific coupling, unit drift, missing attribution or private data forwarded unexpectedly.
- **Validation:** V-DOC, V-APP, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Tracking requires its legal, device and capability gates. Actual devices must prove lifecycle behavior before real collection.
- **Done when:** The stated result and denials pass the listed validation profiles.
