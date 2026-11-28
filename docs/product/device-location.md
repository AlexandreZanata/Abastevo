# Device location: city follow and station pre-selection

Drivers who work from city to city rarely change the selected city by hand.
The Android app uses one device fix to bring the city they are in and, when
they contribute a price photo, the station they are standing at. Manual
selection always stays available and wins.

## B-BR-LOC-01 Station pre-selection when contributing

- Opening the price-photo station picker without a station target (the
  context-free Contribute entry) asks for location permission when it is not
  granted. Denial keeps the manual flow and offers "Use my location" to ask
  again. A target passed by a station page is never replaced.
- With a fix, the app makes one anonymous `GET /v1/stations/nearby` read
  (15 km radius, 10 stations). The nearest station with a real map pin
  (`reviewed`, `osm-mapped`, `anp-reported` or `address-point`; never
  `unknown` or `city-centroid`) no farther than 250 m is pre-selected and
  labelled with its distance.
- Below it, "Not this station? Choose manually" clears the suggestion and
  returns to the manual list; the picker does not suggest again during that
  visit. A station the contributor already picked is never overridden.
- Pre-selection is a convenience only. The submission still carries no
  location ([community-first capture](community/community-first.md)),
  proximity stays `UNKNOWN` and nothing gains trust from it.

## B-BR-LOC-02 City from the device

The device city is the municipality (IBGE code and UF) of the nearest station
within 15 km of the fix that names one, matched in the local IBGE catalog.
No third-party geocoder is called: Nominatim
([ADR-003](../decisions/003-nominatim-reverse-geocode.md)) stays limited to
the explicit "use my location" action in onboarding and the city picker.

## B-BR-LOC-03 City follow

- The first time Explorar opens after the app starts, the app asks for
  location permission when it is not granted and onboarding has not just
  asked; Android may stop showing the dialog after repeated denials. Whenever Explorar comes into view, at most
  once every 15 minutes, a fix makes the device city the preferred city when
  it differs.
- A snackbar names the new city with Undo. Undo restores the previous city and
  stops following until the app process ends. Picking a municipality by hand in
  the city picker also stops following until the app process ends; the
  contribution picker then still lists the stations where the device is, but
  leaves the preferred city alone.
- The contribution picker applies the same follow, so its station list and
  Explorar switch to the same city together.
- Selecting a city still needs imported ANP data (BR-006). Any failure (no
  fix, offline, city outside the catalog) keeps the current city silently.

## B-BR-LOC-04 Privacy

The fix is held in memory only for that lookup. It is never persisted, logged
or sent anywhere other than the nearby read, which keeps no query logs
(risk R02 in [security risks](../security/risks.md)). There is no background
location. Only the resulting city is stored, exactly as a manual choice is.

Code: `DeviceProximityRule` (domain), `FollowDeviceCityUseCase`
(application), `CaptureOcrViewModel` and `HomeViewModel` (app).
